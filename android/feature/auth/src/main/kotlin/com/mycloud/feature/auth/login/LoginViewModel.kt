package com.mycloud.feature.auth.login

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.data.repository.AuthRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class LoginUiState(
    val serverUrl: String = "",
    val email: String = "",
    val password: String = "",
    val isSubmitting: Boolean = false,
    val errorMessage: String? = null,
) {
    val canSubmit: Boolean
        get() = serverUrl.isNotBlank() && email.isNotBlank() && password.isNotBlank() && !isSubmitting
}

@HiltViewModel
class LoginViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(LoginUiState(serverUrl = authRepository.serverUrl()))
    val state: StateFlow<LoginUiState> = _state.asStateFlow()

    fun onServerUrlChange(value: String) = _state.update { it.copy(serverUrl = value, errorMessage = null) }

    fun onEmailChange(value: String) = _state.update { it.copy(email = value, errorMessage = null) }

    fun onPasswordChange(value: String) = _state.update { it.copy(password = value, errorMessage = null) }

    /** On success the repository flips [AuthState]; the root gate navigates away. */
    fun submit() {
        val current = _state.value
        if (!current.canSubmit) return
        _state.update { it.copy(isSubmitting = true, errorMessage = null) }
        viewModelScope.launch {
            // Persist the server address first so the request targets the right host.
            authRepository.setServerUrl(current.serverUrl)
            val result = authRepository.login(current.email.trim(), current.password)
            // Always clear the submitting flag — this ViewModel is Activity-scoped and
            // reused, so leaving it true strands the next visit to Login on a spinner.
            _state.update {
                it.copy(
                    isSubmitting = false,
                    password = if (result is NetworkResult.Success) "" else it.password,
                    errorMessage = if (result is NetworkResult.Success) null else result.userMessageOrNull(),
                )
            }
        }
    }
}
