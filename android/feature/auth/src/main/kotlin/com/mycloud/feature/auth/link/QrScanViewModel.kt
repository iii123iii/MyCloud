package com.mycloud.feature.auth.link

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.data.repository.AuthRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import org.json.JSONObject
import javax.inject.Inject

data class QrScanUiState(
    val phase: Phase = Phase.SCANNING,
    val errorMessage: String? = null,
) {
    enum class Phase { SCANNING, LINKING, ERROR }
}

/**
 * Drives the QR sign-in screen. Parses the scanned payload, claims the code with
 * this device's identity, and waits for the browser to approve. On success the
 * [AuthRepository] flips the session state and the root gate navigates away.
 */
@HiltViewModel
class QrScanViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(QrScanUiState())
    val state: StateFlow<QrScanUiState> = _state.asStateFlow()

    // Guards against the analyzer firing onQrDetected dozens of times per second
    // once a code is in frame — we only act on the first valid one.
    @Volatile
    private var handled = false
    private var linkJob: Job? = null

    fun onQrDetected(raw: String) {
        if (handled) return
        val payload = parse(raw) ?: run {
            _state.update {
                it.copy(
                    phase = QrScanUiState.Phase.ERROR,
                    errorMessage = "That QR code isn't a MyCloud sign-in code.",
                )
            }
            return
        }
        handled = true
        _state.update { it.copy(phase = QrScanUiState.Phase.LINKING, errorMessage = null) }
        linkJob?.cancel()
        linkJob = viewModelScope.launch {
            val result = try {
                authRepository.linkViaQr(payload.url, payload.code, payload.verifier)
            } catch (cancelled: CancellationException) {
                throw cancelled
            }
            if (result !is NetworkResult.Success) {
                handled = false
                _state.update {
                    it.copy(
                        phase = QrScanUiState.Phase.ERROR,
                        errorMessage = result.userMessageOrNull() ?: "Couldn't link this device.",
                    )
                }
            }
            // Success: authState becomes AUTHENTICATED; RootContent leaves this screen.
        }
    }

    fun onScanCanceled() {
        if (handled) return
        _state.update { QrScanUiState() }
    }

    fun onScannerUnavailable(message: String) {
        if (handled) return
        _state.update {
            it.copy(
                phase = QrScanUiState.Phase.ERROR,
                errorMessage = message,
            )
        }
    }

    fun retry() {
        linkJob?.cancel()
        linkJob = null
        handled = false
        _state.update { QrScanUiState() }
    }

    fun cancelAndReset() {
        linkJob?.cancel()
        linkJob = null
        handled = false
        _state.update { QrScanUiState() }
    }

    override fun onCleared() {
        linkJob?.cancel()
        super.onCleared()
    }

    private fun parse(raw: String): QrPayload? = runCatching {
        val o = JSONObject(raw)
        if (o.optInt("v", -1) != 1) return@runCatching null
        val url = o.optString("url").trim()
        val code = o.optString("code").trim()
        val verifier = o.optString("verifier").trim()
        if (url.isEmpty() || code.isEmpty() || verifier.isEmpty() || !isAllowedServerUrl(url)) null
        else QrPayload(url, code, verifier)
    }.getOrNull()

    private fun isAllowedServerUrl(raw: String): Boolean {
        val uri = Uri.parse(raw)
        val scheme = uri.scheme?.lowercase() ?: return false
        val host = uri.host?.lowercase() ?: return false
        return when (scheme) {
            "https" -> true
            "http" -> host.isLocalNetworkHost()
            else -> false
        }
    }

    private fun String.isLocalNetworkHost(): Boolean {
        val host = trim().trim('[', ']').lowercase()
        if (host in LOCAL_HTTP_HOSTS) return true
        if (host.startsWith("192.168.")) return true
        if (host.startsWith("10.")) return true
        val parts = host.split('.')
        if (parts.size == 4 && parts[0] == "172") {
            val second = parts[1].toIntOrNull()
            if (second != null && second in 16..31) return true
        }
        return host.startsWith("fd") || host.startsWith("fe80:")
    }

    private data class QrPayload(val url: String, val code: String, val verifier: String)

    private companion object {
        val LOCAL_HTTP_HOSTS = setOf("localhost", "127.0.0.1", "::1", "10.0.2.2")
    }
}
