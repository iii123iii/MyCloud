package com.mycloud.feature.auth.password

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

data class ChangePasswordUiState(
    val oldPassword: String = "",
    val newPassword: String = "",
    val confirmPassword: String = "",
    val isSubmitting: Boolean = false,
    val errorMessage: String? = null,
    val success: Boolean = false,
) {
    /** True once the user has typed a confirmation that doesn't match the new password. */
    val passwordsMismatch: Boolean
        get() = confirmPassword.isNotEmpty() && newPassword != confirmPassword

    val canSubmit: Boolean
        get() = oldPassword.isNotBlank() &&
            newPassword.length >= MIN_LENGTH &&
            confirmPassword == newPassword &&
            !isSubmitting

    companion object {
        const val MIN_LENGTH = 8
    }
}

@HiltViewModel
class ChangePasswordViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ChangePasswordUiState())
    val state: StateFlow<ChangePasswordUiState> = _state.asStateFlow()

    fun onOldChange(v: String) = _state.update { it.copy(oldPassword = v, errorMessage = null, success = false) }
    fun onNewChange(v: String) = _state.update { it.copy(newPassword = v, errorMessage = null, success = false) }
    fun onConfirmChange(v: String) = _state.update { it.copy(confirmPassword = v, errorMessage = null, success = false) }

    fun submit() {
        val s = _state.value
        if (!s.canSubmit) return
        if (s.newPassword != s.confirmPassword) {
            _state.update { it.copy(errorMessage = "New passwords don't match.") }
            return
        }
        _state.update { it.copy(isSubmitting = true, errorMessage = null) }
        viewModelScope.launch {
            val result = authRepository.changePassword(s.oldPassword, s.newPassword)
            _state.update {
                if (result is NetworkResult.Success) {
                    // Reset fields but flag success so the screen can confirm before the gate navigates away.
                    ChangePasswordUiState(success = true)
                } else {
                    it.copy(isSubmitting = false, errorMessage = result.userMessageOrNull())
                }
            }
        }
    }

    fun logout() {
        viewModelScope.launch { authRepository.logout() }
    }
}
