package com.mycloud.feature.sharing

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.model.Permission
import com.mycloud.data.repository.ShareRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.temporal.ChronoUnit
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject

data class ShareUiState(
    val permission: Permission = Permission.READ,
    val password: String = "",
    val expiresInDays: String = "",
    val downloadLimit: String = "",
    val singleView: Boolean = false,
    val isCreating: Boolean = false,
    val createdLink: String? = null,
    val error: String? = null,
    // "Share with a person" (per-user grant).
    val grantee: String = "",
    val grantPermission: Permission = Permission.READ,
    val isGranting: Boolean = false,
    val grantMessage: String? = null,
    val grantError: String? = null,
)

@HiltViewModel
class ShareViewModel @Inject constructor(
    private val shareRepository: ShareRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ShareUiState())
    val state: StateFlow<ShareUiState> = _state.asStateFlow()

    // Atomic in-flight guards: the UI button disable lags a frame behind a tap, so a
    // double-tap could otherwise launch two requests (two share links / grants).
    private val creating = AtomicBoolean(false)
    private val granting = AtomicBoolean(false)

    fun setPermission(permission: Permission) = _state.update { it.copy(permission = permission) }
    fun setPassword(value: String) = _state.update { it.copy(password = value) }
    fun setExpiresInDays(value: String) = _state.update { it.copy(expiresInDays = value.filter(Char::isDigit)) }
    fun setDownloadLimit(value: String) = _state.update { it.copy(downloadLimit = value.filter(Char::isDigit)) }
    fun setSingleView(value: Boolean) = _state.update { it.copy(singleView = value) }

    fun setGrantee(value: String) = _state.update { it.copy(grantee = value, grantError = null) }
    fun setGrantPermission(permission: Permission) = _state.update { it.copy(grantPermission = permission) }

    /** Reset for a fresh target so a reused (Activity-scoped) VM doesn't show stale state. */
    fun reset() {
        creating.set(false)
        granting.set(false)
        _state.update { ShareUiState() }
    }

    /** Auto-clear the transient "Shared with …" confirmation. */
    fun clearGrantMessage() = _state.update { it.copy(grantMessage = null) }

    /** Grant a person access by username or email. */
    fun addPerson(fileId: String?, folderId: String?) {
        val who = _state.value.grantee.trim()
        if (who.isEmpty()) return
        // Validate before hitting the network: an email needs a plausible address,
        // a bare username must be a sane handle.
        val valid = if (who.contains('@')) {
            EMAIL_REGEX.matches(who)
        } else {
            USERNAME_REGEX.matches(who)
        }
        if (!valid) {
            _state.update { it.copy(grantError = "Enter a valid username or email.") }
            return
        }
        if (!granting.compareAndSet(false, true)) return
        val permission = _state.value.grantPermission
        _state.update { it.copy(isGranting = true, grantError = null, grantMessage = null) }
        viewModelScope.launch {
            try {
                val result = shareRepository.createGrant(fileId, folderId, who, permission)
                when (result) {
                    is NetworkResult.Success ->
                        _state.update { it.copy(isGranting = false, grantee = "", grantMessage = "Shared with $who") }
                    else ->
                        _state.update { it.copy(isGranting = false, grantError = result.userMessageOrNull()) }
                }
            } finally {
                granting.set(false)
            }
        }
    }

    fun create(fileId: String?, folderId: String?) {
        if (!creating.compareAndSet(false, true)) return
        val current = _state.value
        _state.update { it.copy(isCreating = true, error = null) }
        viewModelScope.launch {
            try {
                val expiresIso = current.expiresInDays.toIntOrNull()
                    ?.let { days -> Instant.now().plus(days.toLong(), ChronoUnit.DAYS).toString() }
                val result = shareRepository.createShare(
                    fileId = fileId,
                    folderId = folderId,
                    permission = current.permission,
                    password = current.password,
                    expiresAtIso = expiresIso,
                    downloadLimit = current.downloadLimit.toLongOrNull(),
                    singleView = current.singleView,
                )
                when (result) {
                    is NetworkResult.Success ->
                        _state.update {
                            it.copy(isCreating = false, createdLink = shareRepository.publicLink(result.data.token))
                        }
                    else ->
                        _state.update { it.copy(isCreating = false, error = result.userMessageOrNull()) }
                }
            } finally {
                creating.set(false)
            }
        }
    }

    private companion object {
        val EMAIL_REGEX = Regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$")
        // Letters, digits, and . _ - ; 2–64 chars, no leading/trailing separators.
        val USERNAME_REGEX = Regex("^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9])?$")
    }
}
