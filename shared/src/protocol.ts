export const PROTOCOL_VERSION = "1.0" as const;

export type ProtocolVersion = typeof PROTOCOL_VERSION;
export type MessageType = "auth" | "command" | "frame" | "result" | "log" | "heartbeat";
export type BridgeCommand =
  | "ping"
  | "echo"
  | "connect.discover"
  | "ecu.scan"
  | "dtc.read"
  | "params.read"
  | "tester.present";

export interface Envelope<TPayload = unknown> {
  version: ProtocolVersion;
  ts: string;
  trace_id: string;
  session_id: string;
  msg_type: MessageType;
  payload: TPayload;
}

export interface AuthPayload {
  token: string;
  device_id?: string;
}

export interface CommandPayload {
  command: BridgeCommand;
  args?: Record<string, unknown>;
}

export interface ResultPayload {
  ok: boolean;
  data?: unknown;
  error?: string;
  nrc?: string;
  protocol?: "hsfz" | "doip";
  rtt_ms?: number;
}

export interface LogPayload {
  level: "TRACE" | "DEBUG" | "INFO" | "WARN" | "ERROR" | "CRITICAL";
  module: string;
  event: string;
  message?: string;
  payload_hex?: string;
  result?: string;
  error?: string;
  duration_ms?: number;
}

export interface FramePayload {
  protocol: "hsfz" | "doip" | "uds";
  direction: "tx" | "rx";
  frame_hex: string;
  source?: string;
  target?: string;
  service_id?: string;
  nrc?: string;
  rtt_ms?: number;
  metadata?: Record<string, unknown>;
}

export interface HeartbeatPayload {
  status: "alive";
  uptime_ms?: number;
}

export function createEnvelope<TPayload>(
  input: Omit<Envelope<TPayload>, "version" | "ts"> & { ts?: string },
): Envelope<TPayload> {
  return {
    version: PROTOCOL_VERSION,
    ts: input.ts ?? new Date().toISOString(),
    trace_id: input.trace_id,
    session_id: input.session_id,
    msg_type: input.msg_type,
    payload: input.payload,
  };
}

export function createCommand(
  trace_id: string,
  session_id: string,
  command: CommandPayload["command"],
  args: Record<string, unknown> = {},
): Envelope<CommandPayload> {
  return createEnvelope({
    trace_id,
    session_id,
    msg_type: "command",
    payload: { command, args },
  });
}

export function createAuthEnvelope(
  trace_id: string,
  session_id: string,
  token: string,
  device_id?: string,
): Envelope<AuthPayload> {
  return createEnvelope({
    trace_id,
    session_id,
    msg_type: "auth",
    payload: { token, device_id },
  });
}

export function createFrameEnvelope(
  trace_id: string,
  session_id: string,
  payload: FramePayload,
): Envelope<FramePayload> {
  return createEnvelope({
    trace_id,
    session_id,
    msg_type: "frame",
    payload,
  });
}
