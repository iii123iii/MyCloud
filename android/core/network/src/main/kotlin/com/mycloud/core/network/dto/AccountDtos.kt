package com.mycloud.core.network.dto

import kotlinx.serialization.Serializable

@Serializable
data class SessionDto(
    val jti: String,
    val deviceLabel: String? = null,
    val userAgent: String? = null,
    val ipAddress: String? = null,
    val createdAt: String? = null,
    val lastSeenAt: String? = null,
    val expiresAt: String? = null,
    val isCurrent: Boolean = false,
)

@Serializable
data class SessionsResponse(
    val sessions: List<SessionDto> = emptyList(),
)

@Serializable
data class ActivityDto(
    val id: Long = 0,
    val action: String = "",
    val resourceType: String? = null,
    val resourceId: String? = null,
    val ipAddress: String? = null,
    val createdAt: String? = null,
)

@Serializable
data class ActivityResponse(
    val logs: List<ActivityDto> = emptyList(),
)

@Serializable
data class TokenDto(
    val id: String,
    val name: String = "",
    val tokenPrefix: String? = null,
    val lastFour: String? = null,
    val scopes: List<String> = emptyList(),
    val createdAt: String? = null,
    val expiresAt: String? = null,
    /** Present only in the create response. */
    val token: String? = null,
)

@Serializable
data class TokensResponse(
    val tokens: List<TokenDto> = emptyList(),
)

@Serializable
data class CreateTokenRequestDto(
    val name: String,
    val scopes: List<String> = emptyList(),
)
