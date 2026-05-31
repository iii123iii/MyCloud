package com.mycloud.data.realtime

import com.mycloud.core.network.auth.TokenProvider
import com.mycloud.core.network.realtime.RealtimeClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Owns the realtime connection lifecycle and turns server events into a coarse
 * "data changed" signal that screens collect to refresh. Reconnects with a fixed
 * backoff; correctness still rests on TTL + pull-to-refresh, so this is best-effort.
 */
@Singleton
class RealtimeRepository @Inject constructor(
    private val realtimeClient: RealtimeClient,
    private val tokenProvider: TokenProvider,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var job: Job? = null

    private val _fileEvents = MutableSharedFlow<Unit>(extraBufferCapacity = 16)

    /** Emits whenever a file/folder change event arrives. */
    val fileEvents: SharedFlow<Unit> = _fileEvents.asSharedFlow()

    fun start() {
        if (job?.isActive == true) return
        job = scope.launch {
            while (isActive) {
                val token = tokenProvider.currentAccessToken()
                if (token == null) {
                    delay(RETRY_MILLIS)
                    continue
                }
                runCatching {
                    realtimeClient.events(token).collect { event ->
                        // The hub pushes every data change as type "event" (the
                        // specific file.*/folder.* action is nested in `data`). The
                        // auth ACK is type "auth" — ignore only that. Any "event" is a
                        // coarse "something changed" signal → refresh.
                        if (event.type == "event") {
                            _fileEvents.tryEmit(Unit)
                        }
                    }
                }
                delay(RETRY_MILLIS)
            }
        }
    }

    fun stop() {
        job?.cancel()
        job = null
    }

    private companion object {
        const val RETRY_MILLIS = 3_000L
    }
}
