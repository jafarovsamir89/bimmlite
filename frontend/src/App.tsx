import {
  AlertTriangle,
  ChevronRight,
  Cpu,
  Languages,
  LayoutDashboard,
  LayoutGrid,
  LogOut,
  Menu,
  Power,
  RefreshCw,
  Settings2,
  ShieldAlert,
  SquareStack,
  Sparkles,
  X,
  Zap,
} from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useState } from "react";
import {
  clearEcuDtc,
  connectRead,
  getHealth,
  getLogs,
  readEcuDtc,
  readEcuParams,
} from "./lib/api";

type LogItem = {
  ts: string;
  level: string;
  module: string;
  event: string;
  trace_id: string;
  session_id: string;
  user_id: string;
  vin: string;
  ecu: string;
  duration_ms?: number | null;
  payload_hex: string;
  result: string;
  error: string;
  message: string;
};

type Health = {
  status: string;
  bridge_connected: boolean;
  bridge_attached?: boolean;
  is_alive?: boolean;
  pid?: number;
  last_heartbeat_at?: string | null;
  pending_commands?: number;
};

type EcuInfo = {
  address: string;
  name: string;
  protocol: string;
  present: boolean;
};

type DtcInfo = {
  ecu_address: string;
  ecu_name: string;
  code: string;
  status: string;
  description: string;
  raw: string;
};

type ParameterInfo = {
  ecu_address: string;
  ecu_name: string;
  did: string;
  value_hex: string;
  value_text: string;
};

type Phase1Snapshot = {
  protocol: string;
  vin: string;
  battery_voltage: number | null;
  ecus: EcuInfo[];
  dtcs: DtcInfo[];
  parameters: ParameterInfo[];
};

type Phase1Response = {
  trace_id: string;
  session_id: string;
  snapshot: Phase1Snapshot;
};

type EcuActionResponse = {
  trace_id: string;
  session_id: string;
  ecu_address: string;
  ecu_name: string;
  result: {
    dtcs?: DtcInfo[];
    parameters?: ParameterInfo[];
    result?: string;
    raw?: string;
  };
};

type View = "workspace" | "dashboard" | "settings";
type Locale = "ru" | "en";

type Translation = {
  appName: string;
  workspace: string;
  dashboard: string;
  settings: string;
  signOut: string;
  connected: string;
  connecting: string;
  disconnected: string;
  connect: string;
  disconnect: string;
  waitingTitle: string;
  waitingSubtitle: string;
  vin: string;
  battery: string;
  protocol: string;
  model: string;
  passport: string;
  ecuExplorer: string;
  diagnosticMatrix: string;
  ecuInBlock: string;
  dtcMemory: string;
  dtcReadAll: string;
  dtcClearAll: string;
  aiAnalysis: string;
  readErrors: string;
  clearErrors: string;
  refreshParams: string;
  logs: string;
  liveLogs: string;
  traceFilter: string;
  noData: string;
  noLogs: string;
  health: string;
  ready: string;
  loading: string;
  error: string;
  confirmClear: string;
  confirmClearAll: string;
  readAll: string;
  readSingle: string;
  params: string;
  noFaults: string;
  notScanned: string;
};

const COPY: Record<Locale, Translation> = {
  ru: {
    appName: "BimmLite",
    workspace: "Workspace",
    dashboard: "My Dashboard",
    settings: "Settings",
    signOut: "Выход",
    connected: "CONNECTED",
    connecting: "CONNECTING",
    disconnected: "DISCONNECTED",
    connect: "Connect",
    disconnect: "Disconnect",
    waitingTitle: "Waiting for connection",
    waitingSubtitle: "USE THE CONNECT BUTTON IN THE TOP-RIGHT CORNER",
    vin: "VIN",
    battery: "АКБ",
    protocol: "Протокол",
    model: "Модель",
    passport: "Паспорт авто",
    ecuExplorer: "Control Unit Explorer",
    diagnosticMatrix: "Diagnostic Matrix",
    ecuInBlock: "Войти в блок",
    dtcMemory: "DTC FAULT MEMORY",
    dtcReadAll: "Читать все",
    dtcClearAll: "Стереть все",
    aiAnalysis: "AI-разбор",
    readErrors: "Читать ошибки",
    clearErrors: "Стереть ошибки",
    refreshParams: "Обновить параметры",
    logs: "Логи",
    liveLogs: "Live logs",
    traceFilter: "trace_id",
    noData: "Нет данных",
    noLogs: "Нет логов",
    health: "Статус",
    ready: "Готово",
    loading: "Загрузка...",
    error: "Ошибка",
    confirmClear: "Стереть ошибки этого блока?",
    confirmClearAll: "Стереть ошибки по всем найденным блокам?",
    readAll: "Читать все",
    readSingle: "Читать блок",
    params: "Параметры",
    noFaults: "NO FAULTS",
    notScanned: "NOT SCANNED",
  },
  en: {
    appName: "BimmLite",
    workspace: "Workspace",
    dashboard: "My Dashboard",
    settings: "Settings",
    signOut: "Sign out",
    connected: "CONNECTED",
    connecting: "CONNECTING",
    disconnected: "DISCONNECTED",
    connect: "Connect",
    disconnect: "Disconnect",
    waitingTitle: "Waiting for connection",
    waitingSubtitle: "USE THE CONNECT BUTTON IN THE TOP-RIGHT CORNER",
    vin: "VIN",
    battery: "Battery",
    protocol: "Protocol",
    model: "Model",
    passport: "Vehicle passport",
    ecuExplorer: "Control Unit Explorer",
    diagnosticMatrix: "Diagnostic Matrix",
    ecuInBlock: "Open block",
    dtcMemory: "DTC FAULT MEMORY",
    dtcReadAll: "Read all",
    dtcClearAll: "Clear all",
    aiAnalysis: "AI analysis",
    readErrors: "Read errors",
    clearErrors: "Clear errors",
    refreshParams: "Refresh params",
    logs: "Logs",
    liveLogs: "Live logs",
    traceFilter: "trace_id",
    noData: "No data",
    noLogs: "No logs",
    health: "Status",
    ready: "Ready",
    loading: "Loading...",
    error: "Error",
    confirmClear: "Clear this ECU fault memory?",
    confirmClearAll: "Clear fault memory on every discovered ECU?",
    readAll: "Read all",
    readSingle: "Read ECU",
    params: "Parameters",
    noFaults: "NO FAULTS",
    notScanned: "NOT SCANNED",
  },
};

const DEFAULT_DIDS = ["F186", "F190", "100A", "100E", "172A", "F18C"];

function randomId() {
  return crypto.randomUUID().replaceAll("-", "");
}

function getSessionId() {
  const existing = window.localStorage.getItem("bimmlite.session_id");
  if (existing) {
    return existing;
  }
  const created = `ui-${randomId()}`;
  window.localStorage.setItem("bimmlite.session_id", created);
  return created;
}

function formatVoltage(value: number | null | undefined) {
  if (value == null) {
    return "0.0V";
  }
  return `${value.toFixed(1)}V`;
}

function formatTime(value: string) {
  if (!value) return "--:--:--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value.slice(11, 23);
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function uniqueEcus(ecus: EcuInfo[]) {
  const seen = new Set<string>();
  return ecus.filter((ecu) => {
    if (seen.has(ecu.address)) return false;
    seen.add(ecu.address);
    return true;
  });
}

function decodeVin(vin: string) {
  const normalized = vin.toUpperCase();
  if (normalized.startsWith("WBA3")) {
    return { series: "3 Series", body: "F30" };
  }
  if (normalized.startsWith("WBA1")) {
    return { series: "1 Series", body: "F20" };
  }
  if (normalized.startsWith("WBA2")) {
    return { series: "2 Series", body: "F22" };
  }
  if (normalized.startsWith("WBA5")) {
    return { series: "5 Series", body: "F10" };
  }
  if (normalized.startsWith("WBA7")) {
    return { series: "7 Series", body: "G11" };
  }
  if (normalized.startsWith("WBAK")) {
    return { series: "X Series", body: "F15" };
  }
  const seriesMarker = normalized.slice(0, 4);
  return { series: `${seriesMarker} BMW`, body: "F/G" };
}

function severityFromDtc(dtc: DtcInfo) {
  const text = `${dtc.status} ${dtc.description}`.toLowerCase();
  if (text.includes("critical") || text.includes("high") || dtc.code.startsWith("P0")) {
    return "danger";
  }
  if (text.includes("warn") || dtc.code.startsWith("B") || dtc.code.startsWith("C")) {
    return "warn";
  }
  return "info";
}

function groupByEcu<T extends { ecu_address: string }>(items: T[]) {
  return items.reduce<Record<string, T[]>>((acc, item) => {
    const key = item.ecu_address || "unknown";
    acc[key] = acc[key] ? [...acc[key], item] : [item];
    return acc;
  }, {});
}

function getLevelClass(level: string) {
  const normalized = level.toLowerCase();
  if (normalized === "error" || normalized === "critical") return "error";
  if (normalized === "warn" || normalized === "warning") return "warn";
  if (normalized === "debug") return "debug";
  if (normalized === "trace") return "trace";
  return "info";
}

export default function App() {
  const [locale, setLocale] = useState<Locale>("en");
  const t = COPY[locale];
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [view, setView] = useState<View>("workspace");
  const [health, setHealth] = useState<Health>({ status: "offline", bridge_connected: false });
  const [snapshot, setSnapshot] = useState<Phase1Snapshot | null>(null);
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [loadingSnapshot, setLoadingSnapshot] = useState(false);
  const [notice, setNotice] = useState<string>("");
  const [errorText, setErrorText] = useState<string>("");
  const [logFilter, setLogFilter] = useState("");
  const [selectedEcu, setSelectedEcu] = useState<EcuInfo | null>(null);
  const [matrixOpen, setMatrixOpen] = useState(false);
  const [ecuBusy, setEcuBusy] = useState<string>("");
  const [ecuDetails, setEcuDetails] = useState<Record<string, { dtcs: DtcInfo[]; parameters: ParameterInfo[]; loading: boolean; error: string }>>({});
  const [sessionId] = useState(() => getSessionId());

  useEffect(() => {
    let cancelled = false;
    getHealth()
      .then((value: Health) => {
        if (cancelled) return;
        setHealth(value);
      })
      .catch(() => {
        if (!cancelled) setHealth({ status: "offline", bridge_connected: false });
      });

    getLogs()
      .then((items: LogItem[]) => {
        if (!cancelled) setLogs(items);
      })
      .catch(() => {
        if (!cancelled) setLogs([]);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${wsProtocol}://${window.location.host}/ws/logs`);
    socket.onmessage = (event) => {
      const item = JSON.parse(event.data) as LogItem;
      setLogs((current) => [...current, item].slice(-300));
    };
    return () => socket.close();
  }, []);

  useEffect(() => {
    if (health.bridge_connected && !snapshot && !loadingSnapshot) {
      void refreshSnapshot(true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [health.bridge_connected]);

  const bridgeReady = Boolean(health.bridge_connected && snapshot);
  const ecus = uniqueEcus(snapshot?.ecus ?? []);
  const groupedDtcs = groupByEcu(snapshot?.dtcs ?? []);
  const groupedParams = groupByEcu(snapshot?.parameters ?? []);
  const decoded = decodeVin(snapshot?.vin ?? "");
  const visibleLogs = logs
    .filter((item) => !logFilter || item.trace_id.toLowerCase().includes(logFilter.toLowerCase()))
    .slice()
    .reverse()
    .slice(0, 120);

  async function refreshSnapshot(silent = false) {
    const traceId = randomId();
    setLoadingSnapshot(true);
    setErrorText("");
    if (!silent) {
      setNotice(t.loading);
    }
    try {
      const response = (await connectRead(traceId, sessionId)) as Phase1Response;
      setSnapshot(response.snapshot);
      setHealth((current) => ({ ...current, bridge_connected: true, status: "ok" }));
      setNotice(t.ready);
    } catch (error) {
      const message = error instanceof Error ? error.message : t.error;
      setErrorText(message);
      setNotice("");
    } finally {
      setLoadingSnapshot(false);
    }
  }

  function disconnectDashboard() {
    setSnapshot(null);
    setNotice(t.disconnected);
    setErrorText("");
    setHealth((current) => ({ ...current, bridge_connected: false }));
    setSelectedEcu(null);
    setMatrixOpen(false);
  }

  async function loadEcuDetails(ecu: EcuInfo, force = false) {
    const traceId = randomId();
    const cacheKey = ecu.address;
    if (ecuDetails[cacheKey]?.loading) return;
    setEcuBusy(cacheKey);
    setErrorText("");
    setEcuDetails((current) => ({
      ...current,
      [cacheKey]: {
        dtcs: current[cacheKey]?.dtcs ?? [],
        parameters: current[cacheKey]?.parameters ?? [],
        loading: true,
        error: "",
      },
    }));

    try {
      const [dtcResult, paramResult] = await Promise.allSettled([
        readEcuDtc(traceId, sessionId, ecu.address, ecu.name),
        readEcuParams(traceId, sessionId, ecu.address, ecu.name, DEFAULT_DIDS),
      ]);
      const dtcs =
        dtcResult.status === "fulfilled"
          ? ((dtcResult.value as EcuActionResponse).result?.dtcs ?? [])
          : currentEcuDetails?.dtcs ?? [];
      const parameters =
        paramResult.status === "fulfilled"
          ? ((paramResult.value as EcuActionResponse).result?.parameters ?? [])
          : currentEcuDetails?.parameters ?? [];
      const errors: string[] = [];
      if (dtcResult.status === "rejected") {
        errors.push(dtcResult.reason instanceof Error ? dtcResult.reason.message : t.error);
      }
      if (paramResult.status === "rejected") {
        errors.push(paramResult.reason instanceof Error ? paramResult.reason.message : t.error);
      }
      setEcuDetails((current) => ({
        ...current,
        [cacheKey]: {
          dtcs,
          parameters,
          loading: false,
          error: errors.join(" · "),
        },
      }));
      if (!force) {
        setSelectedEcu(ecu);
      }
    } catch (error) {
      setEcuDetails((current) => ({
        ...current,
        [cacheKey]: {
          dtcs: current[cacheKey]?.dtcs ?? [],
          parameters: current[cacheKey]?.parameters ?? [],
          loading: false,
          error: error instanceof Error ? error.message : t.error,
        },
      }));
      if (!force) {
        setSelectedEcu(ecu);
      }
    } finally {
      setEcuBusy("");
    }
  }

  async function refreshParamsForSelected(ecu: EcuInfo) {
    const traceId = randomId();
    const cacheKey = ecu.address;
    setEcuBusy(cacheKey);
    try {
      const response = (await readEcuParams(traceId, sessionId, ecu.address, ecu.name, DEFAULT_DIDS)) as EcuActionResponse;
      const parameters = response.result?.parameters ?? [];
      setEcuDetails((current) => ({
        ...current,
        [cacheKey]: {
          dtcs: current[cacheKey]?.dtcs ?? [],
          parameters,
          loading: false,
          error: "",
        },
      }));
    } catch (error) {
      setEcuDetails((current) => ({
        ...current,
        [cacheKey]: {
          dtcs: current[cacheKey]?.dtcs ?? [],
          parameters: current[cacheKey]?.parameters ?? [],
          loading: false,
          error: error instanceof Error ? error.message : t.error,
        },
      }));
    } finally {
      setEcuBusy("");
    }
  }

  async function clearSelectedEcu(ecu: EcuInfo) {
    if (!window.confirm(t.confirmClear)) return;
    const traceId = randomId();
    const cacheKey = ecu.address;
    setEcuBusy(cacheKey);
    try {
      await clearEcuDtc(traceId, sessionId, ecu.address, ecu.name);
      await refreshSnapshot(true);
      await loadEcuDetails(ecu, true);
      setNotice(t.ready);
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : t.error);
    } finally {
      setEcuBusy("");
    }
  }

  async function clearAllDtcs() {
    if (!window.confirm(t.confirmClearAll)) return;
    try {
      for (const ecu of ecus) {
        await clearEcuDtc(randomId(), sessionId, ecu.address, ecu.name);
      }
      await refreshSnapshot(true);
      setNotice(t.ready);
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : t.error);
    }
  }

  const currentEcuDetails = selectedEcu ? ecuDetails[selectedEcu.address] : undefined;
  const matrixDtcCount = snapshot?.dtcs.length ?? 0;
  const explorerCount = ecus.length;

  return (
    <div className="app-shell">
      <div className="app-bg" />

      <header className="topbar">
        <div className="topbar__left">
          <button
            className="icon-button"
            type="button"
            aria-label="Open menu"
            onClick={() => setDrawerOpen(true)}
          >
            <Menu size={18} />
          </button>
          <span className={`status-pill status-pill--${health.bridge_connected ? "connected" : "disconnected"}`}>
            <span className="status-pill__dot" aria-hidden="true" />
            {health.bridge_connected ? t.connected : t.disconnected}
          </span>
          <span className="voltage-pill">
            <Zap size={12} />
            {formatVoltage(snapshot?.battery_voltage ?? 0)}
          </span>
          <button className="lang-pill" type="button" onClick={() => setLocale(locale === "en" ? "ru" : "en")}>
            <Languages size={14} />
            {locale.toUpperCase()}
          </button>
        </div>

        <div className="topbar__right">
          <button
            className="connect-button"
            type="button"
            onClick={() => {
              if (bridgeReady) {
                disconnectDashboard();
                return;
              }
              void refreshSnapshot(false);
            }}
            disabled={loadingSnapshot}
          >
            {loadingSnapshot ? <RefreshCw size={16} className="spin" /> : <Power size={16} />}
            <span>{bridgeReady ? t.disconnect : t.connect}</span>
          </button>
        </div>
      </header>

      <main className="content">
        {view === "settings" ? (
          <section className="panel-card panel-card--logs">
            <div className="panel-card__header">
              <div>
                <h2>{t.logs}</h2>
                <p>{t.liveLogs}</p>
              </div>
              <input
                className="log-filter"
                value={logFilter}
                onChange={(event) => setLogFilter(event.target.value)}
                placeholder={t.traceFilter}
                aria-label="Filter logs by trace_id"
              />
            </div>
            <div className="logs-table">
              <div className="logs-table__head">
                <span>ts</span>
                <span>level</span>
                <span>module</span>
                <span>event</span>
                <span>result</span>
              </div>
              {visibleLogs.length ? (
                visibleLogs.map((item) => (
                  <div className="logs-table__row" key={`${item.ts}-${item.event}-${item.trace_id}`}>
                    <span className="mono">{formatTime(item.ts)}</span>
                    <span className={`log-badge log-badge--${getLevelClass(item.level)}`}>{item.level}</span>
                    <span>{item.module}</span>
                    <span>{item.event}</span>
                    <span className="mono">{item.result || item.message || "--"}</span>
                  </div>
                ))
              ) : (
                <div className="empty-state">{t.noLogs}</div>
              )}
            </div>
          </section>
        ) : (
          <>
            <motion.section
              className={`hero-card ${bridgeReady ? "hero-card--ready" : "hero-card--idle"}`}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.35 }}
            >
              <div className="hero-card__watermark" aria-hidden="true">
                <CarHeroMark />
              </div>

              {bridgeReady ? (
                <>
                  <div className="hero-copy">
                    <span className="eyebrow">{t.passport}</span>
                    <h1>{snapshot?.vin ?? t.waitingTitle}</h1>
                    <p>{`${decoded.series} ${decoded.body}`.trim()}</p>
                  </div>

                  <div className="hero-meta">
                    <div className="hero-meta__chip">
                      <span>{t.vin}</span>
                      <strong className="mono">{snapshot?.vin ?? "--"}</strong>
                    </div>
                    <div className="hero-meta__chip">
                      <span>{t.battery}</span>
                      <strong>{formatVoltage(snapshot?.battery_voltage ?? 0)}</strong>
                    </div>
                    <div className="hero-meta__chip">
                      <span>{t.protocol}</span>
                      <strong>{snapshot?.protocol?.toUpperCase() ?? "--"}</strong>
                    </div>
                    <div className="hero-meta__chip">
                      <span>{t.model}</span>
                      <strong>{`${decoded.series} ${decoded.body}`.trim()}</strong>
                    </div>
                  </div>
                </>
              ) : (
                <>
                  <div className="hero-copy">
                    <span className="eyebrow">{t.passport}</span>
                    <h1>{t.waitingTitle}</h1>
                    <p>{t.waitingSubtitle}</p>
                  </div>
                  <div className="hero-meta">
                    <div className="hero-meta__chip hero-meta__chip--placeholder">
                      <span>{t.vin}</span>
                      <strong>--</strong>
                    </div>
                    <div className="hero-meta__chip hero-meta__chip--placeholder">
                      <span>{t.battery}</span>
                      <strong>0.0V</strong>
                    </div>
                    <div className="hero-meta__chip hero-meta__chip--placeholder">
                      <span>{t.protocol}</span>
                      <strong>--</strong>
                    </div>
                    <div className="hero-meta__chip hero-meta__chip--placeholder">
                      <span>{t.model}</span>
                      <strong>BMW F/G</strong>
                    </div>
                  </div>
                </>
              )}
            </motion.section>

            {notice || errorText ? (
              <section className={`status-banner ${errorText ? "status-banner--error" : "status-banner--success"}`}>
                <div>
                  <strong>{errorText ? t.error : t.ready}</strong>
                  <p>{errorText || notice}</p>
                </div>
              </section>
            ) : null}

            <section className="tile-grid">
              <button className="tile tile--warm tile-button" type="button" onClick={() => setMatrixOpen(true)}>
                <div className="tile__icon tile__icon--warm">
                  <AlertTriangle size={20} />
                </div>
                <div className="tile__watermark">
                  <SquareStack size={132} />
                </div>
                <div className="tile__body">
                  <span className="tile__eyebrow">{t.diagnosticMatrix}</span>
                  <h2>{t.dtcMemory}</h2>
                  <div className="tile__metric">
                    <strong>{matrixDtcCount}</strong>
                    <span>{matrixDtcCount ? `${matrixDtcCount} DTC` : t.noFaults}</span>
                  </div>
                </div>
              </button>

              <button className="tile tile--cool tile-button" type="button" onClick={() => ecus[0] && void loadEcuDetails(ecus[0])}>
                <div className="tile__icon tile__icon--cool">
                  <Cpu size={20} />
                </div>
                <div className="tile__watermark">
                  <LayoutGrid size={132} />
                </div>
                <div className="tile__body">
                  <span className="tile__eyebrow">{t.ecuExplorer}</span>
                  <h2>{t.readSingle}</h2>
                  <div className="tile__metric">
                    <strong>{explorerCount}</strong>
                    <span>{snapshot ? `${explorerCount} ECU` : t.notScanned}</span>
                  </div>
                </div>
              </button>
            </section>

            <section className="panel-card">
              <div className="panel-card__header panel-card__header--tight">
                <div>
                  <h2>{t.ecuExplorer}</h2>
                  <p>{bridgeReady ? `${explorerCount} ECU` : t.noData}</p>
                </div>
                <div className="header-actions">
                  <button className="ghost-button" type="button" onClick={() => void refreshSnapshot(false)} disabled={loadingSnapshot}>
                    <RefreshCw size={15} className={loadingSnapshot ? "spin" : ""} />
                    <span>{t.readAll}</span>
                  </button>
                  <button className="ghost-button ghost-button--danger" type="button" onClick={() => setMatrixOpen(true)}>
                    <ShieldAlert size={15} />
                    <span>{t.diagnosticMatrix}</span>
                  </button>
                </div>
              </div>

              {ecus.length ? (
                <div className="ecu-grid">
                  {ecus.map((ecu) => (
                    <button
                      key={ecu.address}
                      type="button"
                      className="ecu-card"
                      onClick={() => {
                        setSelectedEcu(ecu);
                        void loadEcuDetails(ecu);
                      }}
                    >
                      <div className="ecu-card__top">
                        <div>
                          <span className="ecu-card__name">{ecu.name || ecu.address}</span>
                          <span className="ecu-card__sub mono">{ecu.address}</span>
                        </div>
                        <ChevronRight size={16} />
                      </div>
                      <div className="ecu-card__bottom">
                        <span className="ecu-chip">{ecu.protocol || snapshot?.protocol || "--"}</span>
                        <span className={`ecu-chip ${ecu.present ? "ecu-chip--ok" : "ecu-chip--dim"}`}>
                          {ecu.present ? "LIVE" : "OFFLINE"}
                        </span>
                        <span className="ecu-chip">{groupedDtcs[ecu.address]?.length ?? 0} DTC</span>
                      </div>
                    </button>
                  ))}
                </div>
              ) : (
                <div className="empty-state">{bridgeReady ? t.noData : t.loading}</div>
              )}
            </section>
          </>
        )}
      </main>

      <AnimatePresence>
        {drawerOpen ? (
          <motion.div
            className="drawer-backdrop"
            role="presentation"
            onClick={() => setDrawerOpen(false)}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          >
            <motion.aside
              className="drawer"
              role="dialog"
              aria-label="Navigation drawer"
              onClick={(event) => event.stopPropagation()}
              initial={{ x: -18, opacity: 0 }}
              animate={{ x: 0, opacity: 1 }}
              exit={{ x: -18, opacity: 0 }}
            >
              <div className="drawer__top">
                <div className="drawer__brand">
                  <div className="drawer__brand-icon">B</div>
                  <span>{t.appName}</span>
                </div>
                <button className="drawer__close" type="button" onClick={() => setDrawerOpen(false)} aria-label="Close menu">
                  <X size={18} />
                </button>
              </div>

              <nav className="drawer__nav">
                <button
                  type="button"
                  className={`drawer__item ${view === "workspace" ? "drawer__item--active" : ""}`}
                  onClick={() => {
                    setView("workspace");
                    setDrawerOpen(false);
                  }}
                >
                  <LayoutGrid size={16} />
                  <span>{t.workspace}</span>
                </button>
                <button
                  type="button"
                  className={`drawer__item ${view === "dashboard" ? "drawer__item--active" : ""}`}
                  onClick={() => {
                    setView("dashboard");
                    setDrawerOpen(false);
                  }}
                >
                  <LayoutDashboard size={16} />
                  <span>{t.dashboard}</span>
                </button>
                <button
                  type="button"
                  className={`drawer__item ${view === "settings" ? "drawer__item--active" : ""}`}
                  onClick={() => {
                    setView("settings");
                    setDrawerOpen(false);
                  }}
                >
                  <Settings2 size={16} />
                  <span>{t.settings}</span>
                </button>
              </nav>

              <div className="drawer__spacer" />

              <div className="user-card">
                <div className="user-card__avatar">WO</div>
                <div className="user-card__meta">
                  <strong>Workshop Owner</strong>
                  <span>WORKSHOP OWNER</span>
                </div>
              </div>

              <button className="signout-button" type="button" onClick={() => setSnapshot(null)}>
                <LogOut size={16} />
                <span>{t.signOut}</span>
              </button>
            </motion.aside>
          </motion.div>
        ) : null}
      </AnimatePresence>

      <AnimatePresence>
        {selectedEcu ? (
          <motion.div
            className="modal-backdrop"
            role="presentation"
            onClick={() => setSelectedEcu(null)}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          >
            <motion.div
              className="modal"
              role="dialog"
              aria-modal="true"
              aria-label={selectedEcu.name || selectedEcu.address}
              onClick={(event) => event.stopPropagation()}
              initial={{ y: 12, scale: 0.98, opacity: 0 }}
              animate={{ y: 0, scale: 1, opacity: 1 }}
              exit={{ y: 12, scale: 0.98, opacity: 0 }}
            >
              <div className="modal__header">
                <div>
                  <span className="eyebrow">{t.ecuInBlock}</span>
                  <h3>{selectedEcu.name || selectedEcu.address}</h3>
                  <p className="mono">{selectedEcu.address}</p>
                </div>
                <button className="modal__close" type="button" onClick={() => setSelectedEcu(null)}>
                  <X size={18} />
                </button>
              </div>

              <div className="modal__metrics">
                <div className="data-box">
                  <span>{t.protocol}</span>
                  <strong>{selectedEcu.protocol || snapshot?.protocol || "--"}</strong>
                </div>
                <div className="data-box">
                  <span>{t.health}</span>
                  <strong>{selectedEcu.present ? t.ready : t.error}</strong>
                </div>
                <div className="data-box">
                  <span>{t.diagnosticMatrix}</span>
                  <strong>{groupedDtcs[selectedEcu.address]?.length ?? 0}</strong>
                </div>
                <div className="data-box">
                  <span>{t.params}</span>
                  <strong>{groupedParams[selectedEcu.address]?.length ?? 0}</strong>
                </div>
              </div>

              <div className="modal__body">
                <section className="modal-section">
                  <div className="modal-section__header">
                    <h4>{t.dtcMemory}</h4>
                    <button className="ghost-button" type="button" onClick={() => void loadEcuDetails(selectedEcu, true)} disabled={ecuBusy === selectedEcu.address}>
                      <RefreshCw size={14} className={ecuBusy === selectedEcu.address ? "spin" : ""} />
                      <span>{t.readErrors}</span>
                    </button>
                  </div>
                  {currentEcuDetails?.loading ? (
                    <div className="loading-block">{t.loading}</div>
                  ) : currentEcuDetails?.error ? (
                    <div className="error-block">{currentEcuDetails.error}</div>
                  ) : currentEcuDetails?.dtcs?.length ? (
                    <div className="dtc-list">
                      {currentEcuDetails.dtcs.map((dtc) => (
                        <div key={`${dtc.code}-${dtc.raw}-${dtc.ecu_address}`} className={`dtc-row dtc-row--${severityFromDtc(dtc)}`}>
                          <div className="dtc-row__main">
                            <strong className="mono">{dtc.code}</strong>
                            <span>{dtc.description || dtc.status || "--"}</span>
                          </div>
                          <span className="dtc-row__status">{dtc.status || "--"}</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="empty-state">{t.noFaults}</div>
                  )}
                </section>

                <section className="modal-section">
                  <div className="modal-section__header">
                    <h4>{t.params}</h4>
                    <button className="ghost-button" type="button" onClick={() => void refreshParamsForSelected(selectedEcu)} disabled={ecuBusy === selectedEcu.address}>
                      <RefreshCw size={14} className={ecuBusy === selectedEcu.address ? "spin" : ""} />
                      <span>{t.refreshParams}</span>
                    </button>
                  </div>
                  {currentEcuDetails?.parameters?.length ? (
                    <div className="param-list">
                      {currentEcuDetails.parameters.map((param) => (
                        <div key={`${param.did}-${param.value_hex}`} className="param-row">
                          <div className="param-row__left">
                            <strong className="mono">{param.did}</strong>
                            <span>{param.value_text || param.value_hex || "--"}</span>
                          </div>
                          <span className="param-row__right mono">{param.value_hex || "--"}</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="empty-state">{t.noData}</div>
                  )}
                </section>
              </div>

              <div className="modal__footer">
                <button className="ghost-button" type="button" onClick={() => void loadEcuDetails(selectedEcu)}>
                  <ShieldAlert size={15} />
                  <span>{t.readErrors}</span>
                </button>
                <button className="danger-button" type="button" onClick={() => void clearSelectedEcu(selectedEcu)} disabled={ecuBusy === selectedEcu.address}>
                  <AlertTriangle size={15} />
                  <span>{t.clearErrors}</span>
                </button>
              </div>
            </motion.div>
          </motion.div>
        ) : null}
      </AnimatePresence>

      <AnimatePresence>
        {matrixOpen ? (
          <motion.div
            className="modal-backdrop"
            role="presentation"
            onClick={() => setMatrixOpen(false)}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
          >
            <motion.div
              className="modal modal--wide"
              role="dialog"
              aria-modal="true"
              aria-label={t.diagnosticMatrix}
              onClick={(event) => event.stopPropagation()}
              initial={{ y: 12, scale: 0.98, opacity: 0 }}
              animate={{ y: 0, scale: 1, opacity: 1 }}
              exit={{ y: 12, scale: 0.98, opacity: 0 }}
            >
              <div className="modal__header">
                <div>
                  <span className="eyebrow">{t.diagnosticMatrix}</span>
                  <h3>{t.dtcMemory}</h3>
                  <p>{`${matrixDtcCount} DTC`}</p>
                </div>
                <button className="modal__close" type="button" onClick={() => setMatrixOpen(false)}>
                  <X size={18} />
                </button>
              </div>

              <div className="modal__body modal__body--matrix">
                <div className="matrix-toolbar">
                  <button className="ghost-button" type="button" onClick={() => void refreshSnapshot(false)} disabled={loadingSnapshot}>
                    <RefreshCw size={14} className={loadingSnapshot ? "spin" : ""} />
                    <span>{t.dtcReadAll}</span>
                  </button>
                  <button className="ghost-button" type="button" onClick={() => void clearAllDtcs()} disabled={!ecus.length}>
                    <AlertTriangle size={14} />
                    <span>{t.dtcClearAll}</span>
                  </button>
                  <button className="ghost-button ghost-button--disabled" type="button" disabled>
                    <Sparkles size={14} />
                    <span>{t.aiAnalysis}</span>
                  </button>
                </div>

                {ecus.length ? (
                  <div className="matrix-list">
                    {ecus.map((ecu) => {
                      const ecuDtcs = groupedDtcs[ecu.address] ?? [];
                      return (
                        <section key={ecu.address} className="matrix-block">
                          <div className="matrix-block__head">
                            <div>
                              <strong>{ecu.name || ecu.address}</strong>
                              <span className="mono">{ecu.address}</span>
                            </div>
                            <button
                              className="ghost-button"
                              type="button"
                              onClick={() => {
                                setSelectedEcu(ecu);
                                void loadEcuDetails(ecu);
                              }}
                            >
                              <ChevronRight size={14} />
                              <span>{t.readSingle}</span>
                            </button>
                          </div>

                          {ecuDtcs.length ? (
                            <div className="dtc-list dtc-list--matrix">
                              {ecuDtcs.map((dtc) => (
                                <div key={`${ecu.address}-${dtc.code}-${dtc.raw}`} className={`dtc-row dtc-row--${severityFromDtc(dtc)}`}>
                                  <div className="dtc-row__main">
                                    <strong className="mono">{dtc.code}</strong>
                                    <span>{dtc.description || dtc.status || "--"}</span>
                                  </div>
                                  <span className="dtc-row__status">{dtc.status || "--"}</span>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <div className="empty-state">{t.noFaults}</div>
                          )}
                        </section>
                      );
                    })}
                  </div>
                ) : (
                  <div className="empty-state">{t.noData}</div>
                )}
              </div>
            </motion.div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  );
}

function CarHeroMark() {
  return (
    <svg viewBox="0 0 240 240" fill="none" aria-hidden="true">
      <g opacity="0.24">
        <circle cx="120" cy="120" r="88" stroke="currentColor" strokeWidth="1.4" />
        <circle cx="120" cy="120" r="62" stroke="currentColor" strokeWidth="1.2" />
        <circle cx="120" cy="120" r="34" stroke="currentColor" strokeWidth="1.1" />
        <path d="M120 26v188M26 120h188" stroke="currentColor" strokeWidth="1.1" />
        <path d="M48 48l144 144M192 48L48 192" stroke="currentColor" strokeWidth="1" />
      </g>
    </svg>
  );
}
