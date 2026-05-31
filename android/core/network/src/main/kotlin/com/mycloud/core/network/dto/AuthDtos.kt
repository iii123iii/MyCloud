package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

// Field names are camelCase; the global JsonNamingStrategy.SnakeCase maps them to
// the backend's snake_case (e.g. accessToken <-> access_token).

@Serializable
data class LoginRequestDto(
    val email: String,
    val password: String,
)

@Serializable
data class RefreshRequestDto(
    val refreshToken: String,
)

@Serializable
data class LogoutRequestDto(
    val refreshToken: String? = null,
)

@Serializable
data class ChangePasswordRequestDto(
    val oldPassword: String,
    val newPassword: String,
)

@Serializable
data class DeleteAccountRequestDto(
    val password: String,
)

/** Response of POST /auth/login and /auth/refresh. */
@Serializable
data class AuthTokensDto(
    val accessToken: String,
    val refreshToken: String,
    val accessJti: String? = null,
    val refreshJti: String? = null,
    val family: String,
    /** ISO-8601 instant, e.g. "2026-05-29T15:45:00Z". */
    val accessExpiresAt: String,
    val tokenType: String = "Bearer",
    val userId: String,
    val role: String,
    // POST /auth/refresh returns tokens WITHOUT username/email (only /auth/login
    // includes them). These must default or every refresh fails to deserialize —
    // which silently logged the user out at each access-token expiry.
    val username: String = "",
    val email: String = "",
    val mustChangePassword: Boolean = false,
)

/** GET /auth/me. */
@Serializable
data class UserDto(
    val id: String,
    val username: String,
    val email: String,
    val role: String,
    val quotaBytes: Long = 0,
    val usedBytes: Long = 0,
    val isActive: Boolean = true,
    val mustChangePassword: Boolean = false,
    val createdAt: String? = null,
)

/** GET /auth/whoami. */
@Serializable
data class WhoamiDto(
    val id: String,
    val username: String,
    val email: String,
    val role: String,
    val auth: String,
    val scopes: List<String> = emptyList(),
)

@Serializable
data class MessageDto(
    val message: String? = null,
)
