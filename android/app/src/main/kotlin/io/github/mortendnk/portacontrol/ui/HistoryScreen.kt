package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.EnergyReply
import io.github.mortendnk.portacontrol.data.Repository
import io.github.mortendnk.portacontrol.data.Sample

@Composable
fun HistoryScreen(repo: Repository) {
    val devices by repo.devices.collectAsState()
    val list = devices.values.sortedWith(compareBy({ it.order }, { it.name }))
    var selected by remember { mutableStateOf<String?>(null) }
    val id = selected ?: list.firstOrNull()?.id
    var period by remember { mutableStateOf("week") }
    var offset by remember { mutableIntStateOf(0) }
    var hours by remember { mutableIntStateOf(24) }
    var energy by remember { mutableStateOf<EnergyReply?>(null) }
    var samples by remember { mutableStateOf<List<Sample>>(emptyList()) }

    LaunchedEffect(id, period, offset) {
        if (id != null) energy = runCatching { repo.api?.energy(id, period, offset) }.getOrNull()
    }
    LaunchedEffect(id, hours) {
        if (id != null) samples = runCatching { repo.api?.history(id, hours)?.points }.getOrNull().orEmpty()
    }

    Column(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (list.size > 1) {
            Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                list.forEach { d -> FilterChip(selected = d.id == id, onClick = { selected = d.id }, label = { Text(d.name) }) }
            }
        }
        Section(stringResource(R.string.consumption)) {
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                val periods = listOf("day" to R.string.day, "week" to R.string.week, "month" to R.string.month, "year" to R.string.year)
                periods.forEachIndexed { i, (p, l) ->
                    SegmentedButton(
                        selected = period == p,
                        onClick = {
                            period = p
                            offset = 0
                        },
                        shape = SegmentedButtonDefaults.itemShape(i, periods.size),
                        icon = {},
                        label = { Text(stringResource(l), fontSize = 13.sp) },
                    )
                }
            }
            Row {
                IconButton(onClick = { offset-- }) { Icon(Icons.AutoMirrored.Filled.KeyboardArrowLeft, null) }
                Spacer(Modifier.weight(1f))
                IconButton(onClick = { offset++ }, enabled = offset < 0) { Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, null) }
            }
            val e = energy
            if (e != null) {
                Row {
                    StatBox(stringResource(R.string.consumption), num(e.total.kwh, 2) + " kWh", Modifier.weight(1f))
                    StatBox(stringResource(R.string.cost), money(e.total.cost, e.total.currency), Modifier.weight(1f))
                    StatBox(stringResource(R.string.runtime), num(e.total.onHours, 1) + " h", Modifier.weight(1f))
                }
                Bars(e.buckets.map { it.kwh })
            }
        }
        Section(stringResource(R.string.temperatures)) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                listOf(6, 24, 72, 168).forEach { h ->
                    FilterChip(selected = hours == h, onClick = { hours = h }, label = { Text(if (h < 48) "$h h" else "${h / 24} d") })
                }
            }
            val primary = MaterialTheme.colorScheme.primary
            val heat = modeColor("heat")
            val muted = MaterialTheme.colorScheme.onSurfaceVariant
            Lines(samples, listOf<Pair<Color, (Sample) -> Double?>>(primary to { x: Sample -> x.indoor }, heat to { x: Sample -> x.outdoor }, muted to { x: Sample -> if (x.on) x.target else null }))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Legend(primary, stringResource(R.string.indoor))
                Legend(heat, stringResource(R.string.outdoor))
                Legend(muted, stringResource(R.string.target))
            }
            Text(stringResource(R.string.power), fontSize = 14.sp)
            Lines(samples, listOf<Pair<Color, (Sample) -> Double?>>(modeColor("fan") to { x: Sample -> x.watts }), height = 120)
        }
    }
}

@Composable
private fun Legend(c: Color, label: String) {
    Row {
        Canvas(Modifier.size(12.dp).padding(top = 5.dp)) { drawRect(c, size = Size(size.width, 4f)) }
        Spacer(Modifier.size(4.dp))
        Text(label, fontSize = 12.sp)
    }
}

@Composable
private fun Bars(values: List<Double>) {
    val color = MaterialTheme.colorScheme.primary
    val grid = MaterialTheme.colorScheme.outline
    val max = (values.maxOrNull() ?: 0.0).coerceAtLeast(0.01) * 1.1
    Canvas(Modifier.fillMaxWidth().height(160.dp)) {
        drawLine(grid, Offset(0f, size.height), Offset(size.width, size.height))
        if (values.isEmpty()) return@Canvas
        val bw = size.width / values.size
        values.forEachIndexed { i, v ->
            val h = (v / max * size.height).toFloat()
            drawRoundRect(color, Offset(i * bw + bw * 0.15f, size.height - h), Size(bw * 0.7f, h), CornerRadius(4f, 4f))
        }
    }
    Text("max ${num(values.maxOrNull(), 2)} kWh", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

@Composable
private fun Lines(samples: List<Sample>, series: List<Pair<Color, (Sample) -> Double?>>, height: Int = 180) {
    val grid = MaterialTheme.colorScheme.outline
    if (samples.size < 2) {
        Text(stringResource(R.string.no_data), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 13.sp)
        return
    }
    val all = series.flatMap { (_, f) -> samples.mapNotNull(f) }
    var lo = all.minOrNull() ?: 0.0
    var hi = all.maxOrNull() ?: 1.0
    if (hi - lo < 2) {
        lo -= 1
        hi += 1
    }
    val t0 = samples.first().t
    val t1 = samples.last().t
    Canvas(Modifier.fillMaxWidth().height(height.dp)) {
        for (i in 0..3) {
            val y = size.height * i / 3
            drawLine(grid, Offset(0f, y), Offset(size.width, y))
        }
        fun x(t: Long) = ((t - t0).toFloat() / (t1 - t0).coerceAtLeast(1)) * size.width
        fun y(v: Double) = ((1 - (v - lo) / (hi - lo)) * size.height).toFloat()
        series.forEach { (color, f) ->
            val path = Path()
            var pen = false
            var prevT = 0L
            samples.forEach { s ->
                val v = f(s)
                if (v == null || (pen && s.t - prevT > 1800)) pen = false
                if (v != null) {
                    if (!pen) path.moveTo(x(s.t), y(v)) else path.lineTo(x(s.t), y(v))
                    pen = true
                }
                prevT = s.t
            }
            drawPath(path, color, style = Stroke(width = 4f))
        }
    }
    Row {
        Text(num(lo, 0), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.weight(1f))
        Text(num(hi, 0), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
