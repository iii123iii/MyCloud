package com.mycloud.android.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.datastore.settings.AppLockManager
import com.mycloud.core.datastore.settings.SettingsStore
import com.mycloud.core.datastore.settings.ThemeMode
import com.mycloud.core.model.ActivityEntry
import com.mycloud.core.model.PersonalAccessToken
import com.mycloud.core.model.Session
import com.mycloud.core.model.StorageStats
import com.mycloud.core.model.User
import com.mycloud.data.repository.AccountRepository
import com.mycloud.data.repository.AuthRepository
import com.mycloud.data.repository.StorageRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class SettingsUiState(
    val stats: StorageStats? = null,
    val statsLoading: Boolean = false,
    val statsError: String? = null,
    val sessions: List<Session> = emptyList(),
    val activity: List<ActivityEntry> = emptyList(),
    val tokens: List<PersonalAccessToken> = emptyList(),
    val accountError: String? = null,
    val actionError: String? = null,
    val createdTokenSecret: String? = null,
    val createTokenError: String? = null,
    val passwordBusy: Boolean = false,
    val passwordError: String? = null,
    val passwordChanged: Boolean = false,
    val deleteBusy: Boolean = false,
    val deleteError: String? = null,
)

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val storageRepository: StorageRepository,
    private val accountRepository: AccountRepository,
    private val settingsStore: SettingsStore,
    private val appLockManager: AppLockManager,
) : ViewModel() {

    val user: StateFlow<User?> = authRepository.currentUser

    val themeMode: StateFlow<ThemeMode> = settingsStore.themeMode
        .stateIn(viewModelScope, SharingStarted.Eagerly, ThemeMode.SYSTEM)

    val appLockEnabled: StateFlow<Boolean> = appLockManager.enabled
        .stateIn(viewModelScope, SharingStarted.Eagerly, false)

    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state.asStateFlow()

    init {
        loadStats()
        refreshAccount()
    }

    fun loadStats() {
        viewModelScope.launch {
            _state.update { it.copy(statsLoading = true, statsError = null) }
            when (val result = storageRepository.stats()) {
                is NetworkResult.Success ->
                    _state.update { it.copy(stats = result.data, statsLoading = false, statsError = null) }
                else ->
                    _state.update {
                        it.copy(
                            statsLoading = false,
                            statsError = result.userMessageOrNull() ?: "Couldn't load storage usage",
                        )
                    }
            }
        }
    }

    fun refreshAccount() {
        viewModelScope.launch {
            val sessionsResult = accountRepository.sessions()
            val activityResult = accountRepository.activity()
            val tokensResult = accountRepository.tokens()

            // Keep the previously loaded list on a per-call failure rather than blanking it.
            val firstError = listOf(sessionsResult, activityResult, tokensResult)
                .firstOrNull { it !is NetworkResult.Success }
                ?.userMessageOrNull()

            _state.update { prev ->
                prev.copy(
                    sessions = (sessionsResult as? NetworkResult.Success)?.data ?: prev.sessions,
                    activity = (activityResult as? NetworkResult.Success)?.data ?: prev.activity,
                    tokens = (tokensResult as? NetworkResult.Success)?.data ?: prev.tokens,
                    accountError = firstError,
                )
            }
        }
    }

    fun revokeSession(jti: String) {
        viewModelScope.launch {
            when (val result = accountRepository.revokeSession(jti)) {
                is NetworkResult.Success -> {
                    _state.update { it.copy(actionError = null) }
                    refreshAccount()
                }
                else -> _state.update {
                    it.copy(actionError = result.userMessageOrNull() ?: "Couldn't revoke session")
                }
            }
        }
    }

    fun createToken(name: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(createTokenError = null) }
            when (val result = accountRepository.createToken(trimmed, emptyList())) {
                is NetworkResult.Success -> {
                    _state.update { it.copy(createdTokenSecret = result.data, createTokenError = null) }
                    refreshAccount()
                }
                else -> _state.update {
                    it.copy(createTokenError = result.userMessageOrNull() ?: "Couldn't create token")
                }
            }
        }
    }

    fun clearCreatedToken() = _state.update { it.copy(createdTokenSecret = null) }

    fun clearCreateTokenError() = _state.update { it.copy(createTokenError = null) }

    fun clearActionError() = _state.update { it.copy(actionError = null) }

    fun revokeToken(id: String) {
        viewModelScope.launch {
            when (val result = accountRepository.revokeToken(id)) {
                is NetworkResult.Success -> {
                    _state.update { it.copy(actionError = null) }
                    refreshAccount()
                }
                else -> _state.update {
                    it.copy(actionError = result.userMessageOrNull() ?: "Couldn't revoke token")
                }
            }
        }
    }

    fun setTheme(mode: ThemeMode) {
        viewModelScope.launch { settingsStore.setThemeMode(mode) }
    }

    fun setAppLockEnabled(enabled: Boolean) {
        viewModelScope.launch { settingsStore.setAppLockEnabled(enabled) }
    }

    fun changePassword(oldPassword: String, newPassword: String) {
        viewModelScope.launch {
            _state.update { it.copy(passwordBusy = true, passwordError = null, passwordChanged = false) }
            val result = authRepository.changePassword(oldPassword, newPassword)
            _state.update {
                it.copy(
                    passwordBusy = false,
                    passwordChanged = result is NetworkResult.Success,
                    passwordError = if (result is NetworkResult.Success) null
                    else result.userMessageOrNull() ?: "Couldn't change password",
                )
            }
        }
    }

    fun ackPasswordResult() = _state.update {
        it.copy(passwordChanged = false, passwordError = null)
    }

    /** Deletes the account, then clears the local session so the root gate routes to login. */
    fun deleteAccount(password: String) {
        viewModelScope.launch {
            _state.update { it.copy(deleteBusy = true, deleteError = null) }
            when (val result = authRepository.deleteAccount(password)) {
                is NetworkResult.Success -> {
                    // Clear the busy flag before logging out so the dialog doesn't strand on a spinner.
                    _state.update { it.copy(deleteBusy = false) }
                    authRepository.logout()
                }
                else -> _state.update {
                    it.copy(
                        deleteBusy = false,
                        deleteError = result.userMessageOrNull() ?: "Couldn't delete account",
                    )
                }
            }
        }
    }

    fun ackDeleteError() = _state.update { it.copy(deleteError = null) }
}
