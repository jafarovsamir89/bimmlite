from __future__ import annotations

import sqlite3
import zipfile
import zlib
from functools import lru_cache
from pathlib import Path

import structlog

from app.core.config import get_settings


SUPPORTED_DTC_LANGUAGES = {
    "en": "English",
    "de": "Deutsche",
    "ru": "Русский",
    "tr": "Türk",
}


def _normalize_dtc_code(code: str) -> str:
    cleaned = "".join(char for char in (code or "").upper() if char in "0123456789ABCDEF")
    if not cleaned:
        return "00000000"
    if len(cleaned) >= 8:
        return cleaned[-8:]
    return cleaned.rjust(8, "0")


class DtcCatalog:
    def __init__(self, *, source_path: Path, store_path: Path, language: str) -> None:
        self.source_path = source_path
        self.store_path = store_path
        self.language = language if language in SUPPORTED_DTC_LANGUAGES else "en"
        self.logger = structlog.get_logger("dtc_catalog").bind(language=self.language)
        self._initialized = False
        self._missing_source_logged = False

    def describe(self, code: str, ecu_name: str = "") -> str:
        self._ensure_ready()
        normalized = _normalize_dtc_code(code)
        description = self._lookup(normalized)
        if description:
            return description
        if ecu_name:
            return f"{ecu_name}: Fault code {normalized}"
        return f"Diagnostic Trouble Code {normalized}"

    def _ensure_ready(self) -> None:
        if self._initialized:
            return
        self.store_path.parent.mkdir(parents=True, exist_ok=True)
        if not self.store_path.exists() and self.source_path.exists():
            self._build_store_from_source()
        elif not self.source_path.exists() and not self._missing_source_logged:
            self.logger.warning("dtc.source.missing", source_path=str(self.source_path))
            self._missing_source_logged = True
        self._initialized = True

    def _build_store_from_source(self) -> None:
        zip_name = SUPPORTED_DTC_LANGUAGES.get(self.language, SUPPORTED_DTC_LANGUAGES["en"])
        with zipfile.ZipFile(self.source_path) as archive:
            with archive.open(zip_name) as entry:
                raw = entry.read()

        text = zlib.decompress(raw[22:], -15).decode("utf-8", errors="replace")
        conn = sqlite3.connect(self.store_path)
        try:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS dtc_descriptions (
                    lang TEXT NOT NULL,
                    code TEXT NOT NULL,
                    description TEXT NOT NULL,
                    PRIMARY KEY (lang, code)
                )
                """
            )
            conn.execute("DELETE FROM dtc_descriptions WHERE lang = ?", (self.language,))
            rows: list[tuple[str, str, str]] = []
            for line in text.splitlines():
                if "=" not in line:
                    continue
                key, _, description = line.partition("=")
                key = key.strip().upper()
                description = description.strip()
                if not key or not description:
                    continue
                rows.append((self.language, key, description))
            conn.executemany(
                "INSERT OR REPLACE INTO dtc_descriptions(lang, code, description) VALUES (?, ?, ?)",
                rows,
            )
            conn.commit()
            self.logger.info("dtc.store.ready", rows=len(rows), store_path=str(self.store_path))
        finally:
            conn.close()

    def _lookup(self, normalized: str) -> str:
        if not self.store_path.exists():
            return ""

        keys = [normalized.lstrip("0").upper() or "0"]
        six_digit = normalized[2:].lstrip("0").upper() or "0"
        if six_digit not in keys:
            keys.append(six_digit)
        if normalized not in keys:
            keys.append(normalized)

        conn = sqlite3.connect(self.store_path)
        try:
            for key in keys:
                row = conn.execute(
                    "SELECT description FROM dtc_descriptions WHERE lang = ? AND code = ?",
                    (self.language, key),
                ).fetchone()
                if row:
                    return str(row[0])
        finally:
            conn.close()
        return ""


@lru_cache(maxsize=1)
def get_dtc_catalog() -> DtcCatalog:
    settings = get_settings()
    package_root = Path(__file__).resolve().parents[2]
    source_path = Path(settings.dtc_source_path)
    if not source_path.is_absolute():
        source_path = package_root / source_path
    store_path = Path(settings.dtc_store_path)
    if not store_path.is_absolute():
        store_path = package_root / store_path
    return DtcCatalog(source_path=source_path, store_path=store_path, language=settings.dtc_language)
