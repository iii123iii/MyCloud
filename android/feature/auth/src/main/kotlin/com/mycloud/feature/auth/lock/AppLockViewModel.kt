package com.mycloud.feature.auth.lock

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.datastore.settings.AppLockManager
import com.mycloud.data.repository.AuthRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class AppLockViewModel @Inject constructor(
    private val appLockManager: AppLockManager,
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _errorMessage = MutableStateFlow<String?>(null)
    val errorMessage: StateFlow<String?> = _errorMessage.asStateFlow()

    fun unlock() {
        _errorMessage.update { null }
        appLockManager.unlock()
    }

    /** Surface a recoverable error (user cancelled, transient lockout, etc.). */
    fun onError(message: String) = _errorMessage.update { message }

    fun clearError() = _errorMessage.update { null }

    /** Bail out of the locked app entirely when unlock is impossible. */
    fun signOut() {
        viewModelScope.launch {
            authRepository.logout()
            // Clearing the gate lets the root navigate to Login once the session is gone.
            appLockManager.unlock()
        }
    }
}
