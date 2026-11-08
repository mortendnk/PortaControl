package io.github.mortendnk.portacontrol.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

// Mirrors the JSON of /api/v1 (server/internal/app, store, midea).

@Serializable
data class AcState(
    val power: Boolean = false,
    val mode: String = "cool",
    @SerialName("target_temperature") val target: Double = 24.0,
    @SerialName("fan_speed") val fan: Int = 102,
    val swing: Boolean = false,
    val eco: Boolean = false,
    val turbo: Boolean = false,
    val sleep: Boolean = false,
    val purifier: Boolean = false,
    @SerialName("freeze_protection") val freezeProtection: Boolean = false,
    @SerialName("display_on") val displayOn: Boolean = true,
    @SerialName("filter_alert") val filterAlert: Boolean = false,
    @SerialName("indoor_temperature") val indoor: Double? = null,
    @SerialName("outdoor_temperature") val outdoor: Double? = null,
    @SerialName("error_code") val errorCode: Int = 0,
    @SerialName("outdoor_silent") val outdoorSilent: Boolean? = null,
    @SerialName("self_clean") val selfClean: Boolean? = null,
    val ieco: Boolean? = null,
    val sound: Boolean? = null,
    @SerialName("rate_percent") val rate: Int? = null,
    @SerialName("on_timer_minutes") val onTimer: Int? = null,
    @SerialName("off_timer_minutes") val offTimer: Int? = null,
    @SerialName("power_watts") val watts: Double? = null,
    @SerialName("total_energy_kwh") val totalKwh: Double? = null,
)

@Serializable
data class EnergySum(
    val kwh: Double = 0.0,
    val cost: Double = 0.0,
    @SerialName("co2_kg") val co2: Double = 0.0,
    @SerialName("on_hours") val onHours: Double = 0.0,
    val currency: String = "EUR",
)

@Serializable
data class Timers(
    @SerialName("on_at") val onAt: String? = null,
    @SerialName("on_scene_id") val onSceneId: String? = null,
    @SerialName("off_at") val offAt: String? = null,
)

@Serializable
data class Availability(
    val eco: Boolean = true,
    val turbo: Boolean = true,
    @SerialName("outdoor_silent") val outdoorSilent: Boolean = true,
    @SerialName("freeze_protection") val freeze: Boolean = false,
    val sleep: Boolean = true,
    val target: Boolean = true,
    val fan: Boolean = true,
)

@Serializable
data class Device(
    val id: String,
    val name: String,
    val room: String = "",
    val ip: String = "",
    val locked: Boolean = false,
    @SerialName("min_target") val minTarget: Double = 0.0,
    @SerialName("max_target") val maxTarget: Double = 0.0,
    val state: AcState? = null,
    val online: Boolean = false,
    val error: String? = null,
    @SerialName("last_seen") val lastSeen: String? = null,
    val path: String = "",
    val timers: Timers = Timers(),
    val today: EnergySum? = null,
    @SerialName("cost_per_hour") val costPerHour: Double? = null,
    val available: Availability = Availability(),
    val order: Int = 0,
)

/** Partial change; null fields are not sent (explicitNulls = false). */
@Serializable
data class Control(
    val power: Boolean? = null,
    val mode: String? = null,
    @SerialName("target_temperature") val target: Double? = null,
    @SerialName("fan_speed") val fan: Int? = null,
    val swing: Boolean? = null,
    val eco: Boolean? = null,
    val turbo: Boolean? = null,
    val sleep: Boolean? = null,
    val purifier: Boolean? = null,
    @SerialName("freeze_protection") val freezeProtection: Boolean? = null,
    @SerialName("on_timer_minutes") val onTimer: Int? = null,
    @SerialName("off_timer_minutes") val offTimer: Int? = null,
    @SerialName("outdoor_silent") val outdoorSilent: Boolean? = null,
    @SerialName("self_clean") val selfClean: Boolean? = null,
    val ieco: Boolean? = null,
    val sound: Boolean? = null,
    @SerialName("rate_percent") val rate: Int? = null,
    @SerialName("display_on") val display: Boolean? = null,
)

@Serializable
data class Scene(
    val id: String,
    val name: String,
    val icon: String = "",
    val devices: List<String>? = null,
    val control: Control = Control(),
)

@Serializable
data class Action(
    val kind: String,
    @SerialName("scene_id") val sceneId: String? = null,
    val control: Control? = null,
    val text: String? = null,
)

@Serializable
data class Schedule(
    val id: String,
    val name: String,
    val enabled: Boolean,
    val devices: List<String>? = null,
    val days: List<Int>? = null,
    val time: String,
    val action: Action,
    @SerialName("skip_next") val skipNext: Boolean = false,
    @SerialName("next_run") val nextRun: String? = null,
    @SerialName("last_run") val lastRun: String? = null,
)

@Serializable
data class Rule(
    val id: String,
    val name: String,
    val enabled: Boolean,
    val device: String,
    val sensor: String,
    val op: String,
    val value: Double,
    val hysteresis: Double = 0.5,
    val from: String? = null,
    val to: String? = null,
    @SerialName("only_if_off") val onlyIfOff: Boolean = false,
    @SerialName("only_if_on") val onlyIfOn: Boolean = false,
    val action: Action,
    @SerialName("cooldown_minutes") val cooldown: Int = 0,
    @SerialName("last_fired") val lastFired: String? = null,
    val armed: Boolean = true,
)

@Serializable
data class Bucket(
    val start: String,
    val kwh: Double = 0.0,
    val cost: Double = 0.0,
    @SerialName("on_hours") val onHours: Double = 0.0,
    val currency: String = "EUR",
)

@Serializable
data class EnergyReply(
    val period: String = "",
    val buckets: List<Bucket> = emptyList(),
    val total: EnergySum = EnergySum(),
)

@Serializable
data class Sample(
    val t: Long,
    val on: Boolean = false,
    val target: Double = 0.0,
    val indoor: Double? = null,
    val outdoor: Double? = null,
    val watts: Double? = null,
)

@Serializable
data class HistoryReply(val points: List<Sample> = emptyList())

@Serializable
data class LogEntry(
    val id: Long,
    val time: String,
    val device: String? = null,
    val level: String = "info",
    val text: String,
)

@Serializable
data class Health(val ok: Boolean = false, val version: String = "", @SerialName("public_url") val publicUrl: String = "")

@Serializable
data class Me(val role: String = "none", @SerialName("password_set") val passwordSet: Boolean = false)

@Serializable
data class LiveEvent(val type: String, val device: String? = null, val data: JsonElement? = null)

@Serializable
data class Notice(val kind: String = "", val title: String = "", val text: String? = null)
