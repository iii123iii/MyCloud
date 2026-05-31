package com.mycloud.core.network.realtime

import com.mycloud.core.network.di.BaseUrl
import com.mycloud.core.network.di.Unauthenticated
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import javax.inject.Inject
import javax.inject.Singleton

/** A realtime event from the server hub (we only need the type + topic). */
data class RealtimeEvent(val type: String, val topic: String?)

@Serializable
private data class WsEnvelope(
    val type: String = "",
    val topic: String? = null,
)

/**
 * Opens the `/api/v2/ws` socket and emits decoded events. Auth is in-band: on
 * open we send `{"type":"auth","data":"<accessToken>"}` (the bare token, per the
 * server hub). The flow closes on socket failure/closure; reconnect is the
 * caller's responsibility (see RealtimeRepository).
 */
@Singleton
class RealtimeClient @Inject constructor(
    @Unauthenticated private val client: OkHttpClient,
    @BaseUrl private val baseUrl: String,
    private val json: Json,
) {
    fun events(accessToken: String): Flow<RealtimeEvent> = callbackFlow {
        val request = Request.Builder().url("${baseUrl}api/v2/ws").build()
        val listener = object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                webSocket.send("""{"type":"auth","data":"$accessToken"}""")
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                val env = runCatching { json.decodeFromString<WsEnvelope>(text) }.getOrNull()
                if (env != null && env.type.isNotEmpty()) {
                    trySend(RealtimeEvent(env.type, env.topic))
                }
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(NORMAL_CLOSURE, null)
                close()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                close(t)
            }
        }
        val webSocket = client.newWebSocket(request, listener)
        awaitClose { webSocket.cancel() }
    }

    private companion object {
        const val NORMAL_CLOSURE = 1000
    }
}
