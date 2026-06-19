package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

// QR device-linking DTOs. Field names are camelCase; the global
// JsonNamingStrategy.SnakeCase maps them to the backend's snake_case.

@Serializable
data class DeviceLinkClaimRequestDto(
    val code: String,
    val verifier: String,
    val deviceName: String,
    val deviceModel: String,
    val platform: String,
)

@Serializable
data class DeviceLinkPollRequestDto(
    val code: String,
    val verifier: String,
)

/** Response of POST /device-link/claim — just the new state. */
@Serializable
data class DeviceLinkStateDto(
    val state: String = "",
)

/**
 * Response of POST /device-link/poll. Until approved the backend returns only
 * `state`; once approved it returns the full token payload (same shape as
 * /auth/login) and `state` is absent.
 */
@Serializable
data class DeviceLinkPollDto(
    val state: String? = null,
    val accessToken: String? = null,
    val refreshToken: String? = null,
    val accessJti: String? = null,
    val refreshJti: String? = null,
    val family: String? = null,
    val accessExpiresAt: String? = null,
    val userId: String? = null,
    val role: String? = null,
    val username: String = "",
    val email: String = "",
    val mustChangePassword: Boolean = false,
) {
    /** True once the browser approved and tokens were delivered. */
    val hasTokens: Boolean get() = !accessToken.isNullOrEmpty() && !refreshToken.isNullOrEmpty()
}
