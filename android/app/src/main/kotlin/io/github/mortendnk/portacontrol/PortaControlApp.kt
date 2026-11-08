package io.github.mortendnk.portacontrol

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import io.github.mortendnk.portacontrol.data.Repository
import io.github.mortendnk.portacontrol.data.Settings
import io.github.mortendnk.portacontrol.widget.ClimateWidget
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

/** Application with a tiny service locator (no DI framework needed). */
class PortaControlApp : Application() {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    lateinit var settings: Settings
        private set
    lateinit var repo: Repository
        private set

    override fun onCreate() {
        super.onCreate()
        instance = this
        settings = Settings(this)
        repo = Repository(scope)

        scope.launch {
            settings.prefs.distinctUntilChanged().collect { repo.configure(it.serverUrl, it.apiKey) }
        }

        // Live stream only while the app is visible (battery).
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) = repo.setForeground(true)
            override fun onStop(owner: LifecycleOwner) {
                repo.setForeground(false)
                scope.launch { ClimateWidget.updateAll(this@PortaControlApp) }
            }
        })
    }

    companion object {
        lateinit var instance: PortaControlApp
            private set
    }
}
