package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Remove
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.R
import kotlinx.coroutines.delay
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.roundToInt
import kotlin.math.sin

private const val START = 135f
private const val SWEEP = 270f
private const val MIN = 16.0
private const val MAX = 30.0

/** Round thermostat: drag on the ring or use −/+; the value is sent once the user stops. */
@Composable
fun Thermostat(
    value: Double,
    indoor: Double?,
    color: Color,
    enabled: Boolean,
    min: Double = MIN,
    max: Double = MAX,
    onChange: (Double) -> Unit,
) {
    var local by remember { mutableDoubleStateOf(value) }
    var pending by remember { mutableStateOf(false) }
    var dragging by remember { mutableStateOf(false) }

    LaunchedEffect(value) { if (!pending && !dragging) local = value }
    LaunchedEffect(local, pending, dragging) {
        if (pending && !dragging) {
            delay(700)
            pending = false
            if (local != value) onChange(local)
        }
    }

    fun set(v: Double) {
        local = v.coerceIn(min, max).roundToInt().toDouble()
        pending = true
    }

    val track = MaterialTheme.colorScheme.surfaceVariant
    val knobFill = MaterialTheme.colorScheme.surface

    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Box(Modifier.fillMaxWidth().widthIn(max = 280.dp).aspectRatio(1f), contentAlignment = Alignment.Center) {
            Canvas(
                Modifier.fillMaxWidth().aspectRatio(1f)
                    .pointerInput(enabled) {
                        if (!enabled) return@pointerInput
                        fun valueAt(p: Offset): Double {
                            val c = Offset(size.width / 2f, size.height / 2f)
                            var deg = Math.toDegrees(atan2((p.y - c.y).toDouble(), (p.x - c.x).toDouble())).toFloat()
                            if (deg < 0) deg += 360f
                            var rel = deg - START
                            if (rel < 0) rel += 360f
                            if (rel > SWEEP) rel = if (rel > SWEEP + (360f - SWEEP) / 2f) 0f else SWEEP
                            return MIN + rel / SWEEP * (MAX - MIN)
                        }
                        detectDragGestures(
                            onDragStart = {
                                dragging = true
                                set(valueAt(it))
                            },
                            onDragEnd = { dragging = false },
                            onDragCancel = { dragging = false },
                        ) { change, _ -> set(valueAt(change.position)) }
                    }
                    .pointerInput(enabled) {
                        if (!enabled) return@pointerInput
                        detectTapGestures { p ->
                            val c = Offset(size.width / 2f, size.height / 2f)
                            var deg = Math.toDegrees(atan2((p.y - c.y).toDouble(), (p.x - c.x).toDouble())).toFloat()
                            if (deg < 0) deg += 360f
                            var rel = deg - START
                            if (rel < 0) rel += 360f
                            if (rel <= SWEEP) set(MIN + rel / SWEEP * (MAX - MIN))
                        }
                    },
            ) {
                val stroke = size.minDimension * 0.07f
                val r = size.minDimension / 2f - stroke
                val tl = Offset(center.x - r, center.y - r)
                val arcSize = Size(r * 2, r * 2)
                val frac = ((local - MIN) / (MAX - MIN)).toFloat().coerceIn(0f, 1f)
                drawArc(track, START, SWEEP, false, tl, arcSize, style = Stroke(stroke, cap = StrokeCap.Round))
                if (frac > 0.001f) {
                    drawArc(
                        if (enabled) color else color.copy(alpha = 0.4f), START, SWEEP * frac, false, tl, arcSize,
                        style = Stroke(stroke, cap = StrokeCap.Round),
                    )
                }
                val a = Math.toRadians((START + SWEEP * frac).toDouble())
                val k = Offset(center.x + r * cos(a).toFloat(), center.y + r * sin(a).toFloat())
                drawCircle(knobFill, stroke * 0.85f, k)
                drawCircle(color, stroke * 0.85f, k, style = Stroke(stroke * 0.3f))
            }
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text(stringResource(R.string.target).uppercase(), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text("${local.roundToInt()}°", fontSize = 60.sp, fontWeight = FontWeight.SemiBold)
                Text(stringResource(R.string.indoor) + " " + temp(indoor), fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(32.dp)) {
            FilledTonalIconButton(onClick = { set(local - 1) }, enabled = enabled && local > min) {
                Icon(Icons.Filled.Remove, "−1 °C")
            }
            FilledTonalIconButton(onClick = { set(local + 1) }, enabled = enabled && local < max) {
                Icon(Icons.Filled.Add, "+1 °C")
            }
        }
    }
}
