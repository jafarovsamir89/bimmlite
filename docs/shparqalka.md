<aside>
📒

**Правило:** смотрим в **`jafarovsamir89/bimmerapp.com`** → понимаем, как работает → переписываем чисто в BimmLite. bimmerapp — наш репозиторий, поэтому **ДАННЫЕ переносим напрямую, КОД переписываем чисто.** Эта страница — карта: где что лежит и какие значения уже работают (проверено по коду).

</aside>

## 1. Карта файлов bimmerapp (где рабочая логика)

| Что нужно | Файл в bimmerapp |
| --- | --- |
| UDS/DoIP/HSFZ транспорт, VIN, VO/FA, напряжение, SecurityAccess handshake | `backend/services/uds_service.py` |
| ENET discovery, clamp/зажигание, напряжение | `backend/services/enet_service.py` |
| Скан ЭБУ (broadcast) | `backend/services/ecu_scanner.py` |
| Чтение/сброс DTC + парсинг статусов | `backend/services/dtc_reader.py` |
| Адреса/имена ЭБУ, DID, NRC, HSFZ-константы | `backend/services/bmw_constants.py` (ECU_DATABASE, ecu_name, ecu_bus, KNOWN_DIDS, nrc_name) |
| Оркестрация соединения, кэш ЭБУ, VO read/write | `backend/services/connection_manager.py` |
| Живое чтение FDL/CAFD блоков | `backend/services/fdl_reader.py` |
| Декод CAFD → параметры (runtime) | `backend/services/logic_runtime_decoder.py` |
| Оркестрация кодирования (самый большой) | `backend/services/coding_service.py` |
| Транзакции записи (single/multi-block) | `backend/services/coding_transaction_profiles.py` |
| NCD RSA-подпись (i/j/l-правила) | `backend/services/ncd_signature_preprocessor.py` |
| SecurityAccess (провайдеры) | `backend/services/security_access.py`, `psdz_security.py`, `advanced_rsa_security.py` |
| Кодировщик блоков | `backend/services/proprietary_parser/encoder.py` |
| Стандартные функции/оверлей | `runtime_feature_catalog.py`, `standard_feature_store.py`, `standard_overlay_store.py` |
| AI по DTC | `backend/services/dtc_ai_service.py` |

## 2. Проверенные константы и команды (из кода bimmerapp)

**Порты/адреса:**

- HSFZ (F-серия): TCP **6801**; DoIP (G): TCP **13400**; ENET discovery: UDP **6811**.
- Тестер = **0xF4**, шлюз ZGW = **0x10**, функциональный broadcast = **0xDF**.
- VIN читать с адресов (по порядку): **0x40 → 0x10 → 0xDF**.

**HSFZ-кадр (проверено):** тело = `[0xF4][target][UDS...]`, заголовок = `[4 байта len][2 байта CW=0x0001]`:

```python
payload = bytes([0xF4, target]) + uds_payload
packet  = struct.pack("!LH", len(payload), 0x0001) + payload
```

**VIN:** UDS `22 F1 90` → ответ `62 F1 90 <17 ASCII>`.

**Скан ЭБУ (broadcast):** `f4 df 3E 00 01` (TesterPresent на 0xDF) → собрать адреса ответивших (первый байт ответа, <0xDF, пропуск ACK), имена — через ECU_DATABASE.

**DTC:** запрос `19 02 2F` (маска реальных неисправностей). Ответ `59 02 <availabilityMask> <DTC...>`; **пропустить mask-байт**, далее записи по 4 байта (3 code + 1 status). Сброс: `14 FF FF FF`.

**Напряжение АКБ:** DID `DA D8` (IHKA/шлюз); если 80..170 → /10 = вольты. Fallback — F101 BCD.

**NRC обработка:** `0x78` (pending) — продолжать чтение; `0x37`/`0x36` — ждать ~11с и повтор; `0x35` (invalidKey) — неверный ключ/magic. TesterPresent = `3E 80` (suppress response).

**Сессии:** `10 01` default, `10 03` extended, `10 41` coding.

## 3. Данные к переносу (копируем напрямую, они наши)

| Данные | Файл в bimmerapp → куда в BimmLite |
| --- | --- |
| Расшифрованные кодировки | `backend/data/ecu_logic_v2.sqlite` → `catalog` |
| Стандартные функции + overlay | `standard_features.v1.sqlite`, `standard_overlay.v1.sqlite`, `logic_name_overlay.v1.json` → `catalog` |
| RSA / SecurityAccess ключи | `sa_keys_l3.json`, `sa_keys_v2.json`, `security_keys.db` → `security` (только сервер) |
| SALAPA-маппинг | `backend/data/salapa_mapping.json` → `catalog` |
| Каталог параметров/пресетов | `coding_catalog.sqlite` (через `data/coding_db.py`) → `catalog` |
| Кодировочные параметры (encoder) | `backend/data/AiCoder_Params.json` → `catalog` |
| DTC descriptions | сервис `dtc_description_service.py`  • его источник → `dtc` |

<aside>
⚠️

Ключи/секреты переносим только на сервер (`security`), НЕ в git и НЕ на мост. Проверить, что RSA-ключи не попадают в репозиторий.

</aside>

## 4. Знания по кодированию (для Фазы 4)

- **CAFD-блоки** кодирования: DID диапазон **0x3000–0x36FF**.
- **NCD-подпись** (без пересчёта ECU откатит CAFD в virgin 0xFF): две схемы — **Type A: CAF_ID 0x37F0 + NCD_SIG 0x37F1** (KOMBI); **Type B: 0x37FC + 0x37FD** (FEM/BDC). CPS = **0x37FE**.
- **Транзакции:** `single_block_write_v1` (универсальный) и `multi_block_commit_v1` (для ECU с NCD-подписью). См. `coding_transaction_profiles.py`.
- **SecurityAccess:** Local (`key = ROL32(seed,1) XOR magic` по ECU) и PSDZ RSA-1024 (MD5 → byteswap → raw RSA → 132 байта) — ключ по BTLD SGBM-ID. См. `security_access.py` / `psdz_security.py`.
- **VO/FA:** чтение DID `3F06` (мета) + `3F09` (бинарный FA с SA-кодами); запись FA — DID `1000` (WriteDataByIdentifier). См. `connection_manager.py` / `uds_service.get_vehicle_order`.

## 5. Анти-паттерны bimmerapp (чего НЕ повторять)

- **3 провайдера SecurityAccess** в разных файлах → сделать **один** модуль с единым интерфейсом.
- **Несколько путей чтения VO/FA** → один каноничный путь.
- **UDS-логика размазана** по uds_service + fdl_reader + dtc_reader + ecu_scanner → единый `uds`/`transport`.
- **Разрозненные базы ключей** (`sa_keys_l3` + `sa_keys_v2` + `security_keys.db`) → единый `security` store.
- **Разные форматы бэкапа/снимка** → один формат.