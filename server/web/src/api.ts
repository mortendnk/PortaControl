// Typed client for /api/v1 (see docs/API.md).

export type Mode = "auto" | "cool" | "dry" | "heat" | "fan";

export interface State {
  power: boolean;
  mode: Mode;
  target_temperature: number;
  fan_speed: number;
  swing: boolean;
  eco: boolean;
  turbo: boolean;
  sleep: boolean;
  fahrenheit: boolean;
  follow_me: boolean;
  purifier: boolean;
  freeze_protection: boolean;
  display_on: boolean;
  filter_alert: boolean;
  indoor_temperature: number | null;
  outdoor_temperature: number | null;
  error_code: number;
  outdoor_silent: boolean | null;
  self_clean: boolean | null;
  ieco: boolean | null;
  sound: boolean | null;
  rate_percent: number | null;
  on_timer_minutes: number | null;
  off_timer_minutes: number | null;
  power_watts: number | null;
  total_energy_kwh: number | null;
  run_energy_kwh: number | null;
}

export interface EnergySum {
  kwh: number;
  cost: number;
  co2_kg: number;
  on_hours: number;
  avg_indoor?: number;
  max_watts: number;
  currency: string;
}

export interface Timers {
  on_at: string | null;
  on_scene_id?: string;
  off_at: string | null;
}

export interface Availability {
  eco: boolean;
  turbo: boolean;
  outdoor_silent: boolean;
  freeze_protection: boolean;
  sleep: boolean;
  target: boolean;
  fan: boolean;
}

export interface Device {
  id: string;
  name: string;
  room: string;
  ip: string;
  port: number;
  sn?: string;
  connection: "auto" | "lan" | "cloud";
  order: number;
  favorite: boolean;
  simulated?: boolean;
  created_at: string;
  min_target?: number;
  max_target?: number;
  locked?: boolean;
  state: State | null;
  online: boolean;
  error?: string;
  last_seen: string | null;
  path: string;
  timers: Timers;
  has_lan: boolean;
  has_cloud: boolean;
  today?: EnergySum;
  cost_per_hour?: number;
  available: Availability;
}

export interface Control {
  power?: boolean;
  mode?: Mode;
  target_temperature?: number;
  fan_speed?: number;
  swing?: boolean;
  eco?: boolean;
  turbo?: boolean;
  sleep?: boolean;
  purifier?: boolean;
  freeze_protection?: boolean;
  fahrenheit?: boolean;
  on_timer_minutes?: number;
  off_timer_minutes?: number;
  outdoor_silent?: boolean;
  self_clean?: boolean;
  ieco?: boolean;
  sound?: boolean;
  rate_percent?: number;
  display_on?: boolean;
}

export interface Scene {
  id: string;
  name: string;
  icon: string;
  devices: string[] | null;
  control: Control;
  order: number;
}

export interface Action {
  kind: "on" | "off" | "scene" | "control" | "notify";
  scene_id?: string;
  control?: Control;
  text?: string;
}

export interface Schedule {
  id: string;
  name: string;
  enabled: boolean;
  devices: string[] | null;
  days: number[] | null;
  time: string;
  action: Action;
  last_run?: string;
  skip_next: boolean;
  next_run?: string | null;
}

export interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  device: string;
  sensor: "indoor" | "outdoor" | "power_watts";
  op: "above" | "below";
  value: number;
  hysteresis: number;
  from?: string;
  to?: string;
  only_if_off?: boolean;
  only_if_on?: boolean;
  action: Action;
  cooldown_minutes: number;
  last_fired?: string;
}

export interface Sample {
  t: number;
  on: boolean;
  mode: string;
  target: number;
  indoor: number | null;
  outdoor: number | null;
  watts: number | null;
  kwh: number | null;
  fan: number;
}

export interface Bucket extends EnergySum {
  start: string;
}

export interface EnergyReply {
  period: string;
  offset: number;
  from: string;
  to: string;
  buckets: Bucket[];
  total: EnergySum;
}

export interface Diagnostics {
  compressor_hz: number | null;
  compressor_target_hz: number | null;
  current_a: number | null;
  voltage_v: number | null;
  t1_indoor_air: number | null;
  t2_indoor_coil: number | null;
  indoor_temp2: number | null;
  t3_outdoor_coil: number | null;
  t4_outdoor_air: number | null;
  tp_discharge: number | null;
  indoor_fan_rpm: number | null;
  indoor_fan_target_rpm: number | null;
  condensate_pump: boolean | null;
  eev_opening: number | null;
  outdoor_fan_rpm: number | null;
  defrost: boolean | null;
  humidity: number | null;
  compressor_power_w: number | null;
  current_run_kwh: number | null;
  energy_counter_raw: number | null;
  identifier?: string;
  raw: Record<string, string>;
}

export interface Webhook {
  id: string;
  url: string;
  events: string[] | null;
  enabled: boolean;
}

export interface Settings {
  price_per_kwh: number;
  currency: string;
  poll_seconds: number;
  fast_poll_seconds: number;
  idle_disconnect_seconds: number;
  sample_seconds: number;
  history_days: number;
  guest_control: boolean;
  language: string;
  temperature_unit: string;
  beep: boolean;
  cloud: { account: string };
  mqtt: { enabled: boolean; broker: string; username: string; password?: string; prefix: string; discovery: boolean; discovery_prefix: string };
  webhooks: Webhook[] | null;
  notify: {
    ntfy_url: string;
    filter_alert: boolean;
    error_code: boolean;
    offline: boolean;
    automation: boolean;
    temp_alarm_max?: number | null;
    temp_alarm_min?: number | null;
  };
  co2_grams_per_kwh: number;
  monthly_budget_kwh: number;
}

export interface LogEntry {
  id: number;
  time: string;
  device?: string;
  level: "info" | "warn" | "error";
  text: string;
}

export interface Discovered {
  ip: string;
  port: number;
  device_id: string;
  sn: string;
  name: string;
  type: number;
  version: number;
  known: boolean;
  air_conditioner: boolean;
}

export interface CloudAppliance {
  device_id: string;
  name: string;
  type: number;
  online: boolean;
}

export interface Me {
  role: "admin" | "guest" | "none";
  password_set: boolean;
  guest_control: boolean;
  private_network: boolean;
}

export class ApiError extends Error {
  status: number;
  body: any;
  constructor(status: number, message: string, body?: any) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

export async function api<T = any>(path: string, opts: { method?: string; body?: any; raw?: boolean } = {}): Promise<T> {
  const init: RequestInit = {
    method: opts.method ?? (opts.body !== undefined ? "POST" : "GET"),
    headers: { "X-Client": "Web" },
    credentials: "same-origin",
  };
  if (opts.body !== undefined) {
    if (opts.raw) {
      init.body = opts.body;
    } else {
      (init.headers as Record<string, string>)["Content-Type"] = "application/json";
      init.body = JSON.stringify(opts.body);
    }
  }
  const res = await fetch("/api/v1" + path, init);
  const text = await res.text();
  let data: any = undefined;
  try {
    data = text ? JSON.parse(text) : undefined;
  } catch {
    data = text;
  }
  if (!res.ok) {
    throw new ApiError(res.status, (data && data.error) || res.statusText, data);
  }
  return data as T;
}

export const control = (id: string, c: Control) => api<Device>(`/devices/${id}/control`, { body: c });
