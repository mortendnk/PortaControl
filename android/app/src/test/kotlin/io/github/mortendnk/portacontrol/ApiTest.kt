package io.github.mortendnk.portacontrol

import io.github.mortendnk.portacontrol.data.AcState
import io.github.mortendnk.portacontrol.data.Api
import io.github.mortendnk.portacontrol.data.Control
import io.github.mortendnk.portacontrol.data.Device
import io.github.mortendnk.portacontrol.data.apply
import io.github.mortendnk.portacontrol.data.json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ApiTest {
    @Test
    fun normalize() {
        assertEquals("http://192.168.1.5:8080", Api.normalize("192.168.1.5"))
        assertEquals("http://192.168.1.5:9000", Api.normalize("192.168.1.5:9000/"))
        assertEquals("https://pc.example.org", Api.normalize("https://pc.example.org/"))
    }

    @Test
    fun parseLink() {
        val p = Api.parseLink("portacontrol://connect?url=http%3A%2F%2F10.0.0.2%3A8080&key=pc_abc")
        assertEquals("http://10.0.0.2:8080" to "pc_abc", p)
        assertNull(Api.parseLink("https://example.org"))
    }

    @Test
    fun controlOmitsNulls() {
        val s = json.encodeToString(Control.serializer(), Control(power = true, target = 23.0))
        assertEquals("""{"power":true,"target_temperature":23.0}""", s)
    }

    @Test
    fun decodeDevice() {
        val raw = """{"id":"1","name":"Wohnzimmer","state":{"power":true,"mode":"heat","target_temperature":21,
            "fan_speed":102,"indoor_temperature":19.5,"outdoor_temperature":null,"rate_percent":100,"unknown":1},
            "online":true,"timers":{"on_at":null,"off_at":null},"available":{"eco":false}}"""
        val d = json.decodeFromString(Device.serializer(), raw)
        assertEquals("heat", d.state?.mode)
        assertEquals(19.5, d.state?.indoor ?: 0.0, 0.001)
        assertFalse(d.available.eco)
    }

    @Test
    fun optimisticApply() {
        val s = AcState(power = false, mode = "cool").apply(Control(power = true, mode = "heat"))
        assertTrue(s.power)
        assertEquals("heat", s.mode)
    }
}
