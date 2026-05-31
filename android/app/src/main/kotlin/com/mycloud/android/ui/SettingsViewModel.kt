package com.mycloud.android.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
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
    val sessions: List<Session> = emptyList(),
    val activity: List<ActivityEntry> = emptyList(),
    val tokens: List<PersonalAccessToken> = emptyList(),
    val createdTokenSecret: String? = null,
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
) : ViewModel() {

    val user: StateFlow<User?> = authRepository.currentUser

    val themeMode: StateFlow<ThemeMode> = settingsStore.themeMode
        .stateIn(viewModelScope, SharingStarted.Eagerly, ThemeMode.SYSTEM)

    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state.asStateFlow()

    init {
        viewModelScope.launch {
            (storageRepository.stats() as? NetworkResult.Success)?.data?.let { stats ->
                _state.update { it.copy(stats = stats) }
            }
        }
        refreshAccount()
    }

    fun refreshAccount() {
        viewModelScope.launch {
            val sessions = (accountRepository.sessions() as? NetworkResult.Success)?.data.orEmpty()
            val activity = (accountRepository.activity() as? NetworkResult.Success)?.data.orEmpty()
            val tokens = (accountRepository.tokens() as? NetworkResult.Success)?.data.orEmpty()
            _state.update { it.copy(sessions = sessions, activity = activity, tokens = tokens) }
        }
    }

    fun revokeSession(jti: String) {
        viewModelScope.launch {
            accountRepository.revokeSession(jti)
            refreshAccount()
        }
    }

    fun createToken(name: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch {
            val result = accountRepository.createToken(trimmed, emptyList())
            if (result is NetworkResult.Success) {
                _state.update { it.copy(createdTokenSecret = result.data) }
            }
            refreshAccount()
        }
    }

    fun clearCreatedToken() = _state.update { it.copy(createdTokenSecret = null) }

    fun revokeToken(id: String) {
        viewModelScope.launch {
            accountRepository.revokeToken(id)
            refreshAccount()
        }
    }

    fun setTheme(mode: ThemeMode) {
        viewModelScope.launch { settingsStore.setThemeMode(mode) }
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
                is NetworkResult.Success -> authRepository.logout()
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
