from __future__ import annotations

import zipfile
import zlib
from pathlib import Path

from app.services.dtc_catalog import DtcCatalog


def _write_dtc_dat(path: Path, lines: str) -> None:
    compressor = zlib.compressobj(level=9, wbits=-15)
    payload = compressor.compress(lines.encode("utf-8")) + compressor.flush()
    vendor_blob = (b"\x00" * 22) + payload

    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_STORED) as archive:
        archive.writestr("English", vendor_blob)


def test_dtc_catalog_builds_store_and_resolves_description(tmp_path: Path) -> None:
    source_path = tmp_path / "DTC.dat"
    store_path = tmp_path / "dtc.sqlite"
    _write_dtc_dat(source_path, "123456=Charge pressure plausibility\nABCDEF=Steering angle invalid\n")

    catalog = DtcCatalog(source_path=source_path, store_path=store_path, language="en")

    assert catalog.describe("123456") == "Charge pressure plausibility"
    assert catalog.describe("00ABCDEF") == "Steering angle invalid"


def test_dtc_catalog_falls_back_without_source(tmp_path: Path) -> None:
    catalog = DtcCatalog(source_path=tmp_path / "missing.dat", store_path=tmp_path / "missing.sqlite", language="en")

    assert catalog.describe("2A6B00", ecu_name="DSC") == "DSC: Fault code 002A6B00"
