package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.model.AuthState
import com.mycloud.core.model.User
import kotlinx.coroutines.flow.StateFlow

/** Owns the session: login, restore, password change, logout, and the observable
 *  [authState] / [currentUser] that drive the root navigation gate. */
interface AuthRepository {

    val authState: StateFlow<AuthState>
    val currentUser: StateFlow<User?>

    /** The configured backend base URL (for pre-filling the login form). */
    fun serverUrl(): String

    /** Persist the backend base URL; takes effect immediately for new requests. */
    suspend fun setServerUrl(url: String)

    /** Load any persisted session and validate it against GET /auth/me. */
    suspend fun restoreSession()

    suspend fun login(email: String, password: String): NetworkResult<Unit>

    suspend fun changePassword(oldPassword: String, newPassword: String): NetworkResult<Unit>

    /** Permanently delete the account after re-confirming the current password.
     *  On success the caller should follow up with [logout] to clear local state. */
    suspend fun deleteAccount(password: String): NetworkResult<Unit>

    suspend fun logout()
}
