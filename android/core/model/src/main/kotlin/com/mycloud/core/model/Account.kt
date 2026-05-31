package com.mycloud.core.model

/** An active session/device (GET /me/sessions). */
data class Session(
    val jti: String,
    val deviceLabel: String?,
    val ipAddress: String?,
    val lastSeenAtMillis: Long,
    val isCurrent: Boolean,
)

/** An activity-log entry (GET /auth/me/activity). */
data class ActivityEntry(
    val id: String,
    val action: String,
    val resourceType: String?,
    val ipAddress: String?,
    val createdAtMillis: Long,
)

/** A personal access token (GET /me/tokens). The secret is only returned on create. */
data class PersonalAccessToken(
    val id: String,
    val name: String,
    val tokenPrefix: String?,
    val lastFour: String?,
    val scopes: List<String>,
    val createdAtMillis: Long,
    val expiresAtMillis: Long?,
)
