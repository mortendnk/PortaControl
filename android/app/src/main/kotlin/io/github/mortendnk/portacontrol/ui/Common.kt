package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AcUnit
import androidx.compose.material.icons.filled.Air
import androidx.compose.material.icons.filled.LocalFireDepartment
import androidx.compose.material.icons.filled.WaterDrop
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.R
import java.text.NumberFormat
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

data class ModeDef(val id: String, val label: Int, val icon: ImageVector)

val MODES = listOf(
    ModeDef("cool", R.string.mode_cool, Icons.Filled.AcUnit),
    ModeDef("heat", R.string.mode_heat, Icons.Filled.LocalFireDepartment),
    ModeDef("dry", R.string.mode_dry, Icons.Filled.WaterDrop),
    ModeDef("fan", R.string.mode_fan, Icons.Filled.Air),
    ModeDef("auto", R.string.mode_auto, Icons.Filled.Autorenew),
)

fun modeIcon(mode: String?): ImageVector = MODES.firstOrNull { it.id == mode }?.icon ?: Icons.Filled.Autorenew

val FAN_PRESETS = listOf(20 to R.string.fan_silent, 40 to R.string.fan_low, 60 to R.string.fan_medium, 80 to R.string.fan_high, 100 to R.string.fan_max)

fun num(v: Double?, digits: Int = 1): String {
    if (v == null) return "–"
    val f = NumberFormat.getNumberInstance(Locale.getDefault())
    f.minimumFractionDigits = digits
    f.maximumFractionDigits = digits
    return f.format(v)
}

fun temp(v: Double?): String = if (v == null) "–" else num(v) + " °C"

fun money(v: Double?, currency: String = "EUR"): String {
    if (v == null) return "–"
    return runCatching {
        NumberFormat.getCurrencyInstance(Locale.getDefault()).apply { this.currency = java.util.Currency.getInstance(currency) }.format(v)
    }.getOrElse { num(v, 2) + " " + currency }
}

fun clock(iso: String?): String {
    if (iso.isNullOrBlank()) return "–"
    return runCatching {
        val i = Instant.parse(iso).atZone(ZoneId.systemDefault())
        val today = java.time.LocalDate.now()
        val fmt = DateTimeFormatter.ofPattern("HH:mm")
        when (i.toLocalDate()) {
            today -> i.format(fmt)
            today.plusDays(1) -> "morgen " + i.format(fmt)
            else -> i.format(DateTimeFormatter.ofPattern("EEE dd.MM. HH:mm"))
        }
    }.getOrElse { iso }
}

@Composable
fun Section(title: String? = null, modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(20.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            if (title != null) Text(title, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
            content()
        }
    }
}

@Composable
fun StatBox(label: String, value: String, modifier: Modifier = Modifier, sub: String? = null) {
    Column(
        modifier
            .padding(2.dp),
    ) {
        Text(label.uppercase(), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, fontSize = 18.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
        if (sub != null) Text(sub, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
    }
}

@Composable
fun LabeledRow(label: String, hint: String? = null, content: @Composable () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(label, fontSize = 14.sp)
            if (hint != null) Text(hint, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        content()
    }
}
