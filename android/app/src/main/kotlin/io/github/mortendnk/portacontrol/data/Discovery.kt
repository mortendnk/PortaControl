package io.github.mortendnk.portacontrol.data

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow

data class FoundServer(val name: String, val url: String)

/** Finds PortaControl servers on the LAN (mDNS service _portacontrol._tcp, announced by avahi in the LXC). */
fun discoverServers(context: Context): Flow<FoundServer> = callbackFlow {
    val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager

    fun hostOf(info: NsdServiceInfo): String? =
        if (Build.VERSION.SDK_INT >= 34) {
            info.hostAddresses.firstOrNull { it is java.net.Inet4Address }?.hostAddress
                ?: info.hostAddresses.firstOrNull()?.hostAddress
        } else {
            @Suppress("DEPRECATION")
            info.host?.hostAddress
        }

    val listener = object : NsdManager.DiscoveryListener {
        override fun onDiscoveryStarted(serviceType: String) {}
        override fun onDiscoveryStopped(serviceType: String) {}
        override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
            close()
        }
        override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) {}
        override fun onServiceLost(serviceInfo: NsdServiceInfo) {}
        override fun onServiceFound(serviceInfo: NsdServiceInfo) {
            @Suppress("DEPRECATION")
            nsd.resolveService(serviceInfo, object : NsdManager.ResolveListener {
                override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {}
                override fun onServiceResolved(info: NsdServiceInfo) {
                    val host = hostOf(info) ?: return
                    val h = if (host.contains(':')) "[$host]" else host
                    trySend(FoundServer(info.serviceName, "http://$h:${info.port}"))
                }
            })
        }
    }

    nsd.discoverServices("_portacontrol._tcp", NsdManager.PROTOCOL_DNS_SD, listener)
    awaitClose { runCatching { nsd.stopServiceDiscovery(listener) } }
}
