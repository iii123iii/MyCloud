package com.mycloud.data.mapper

import com.mycloud.core.datastore.token.TokenBundle
import com.mycloud.core.model.User
import com.mycloud.core.model.UserRole
import com.mycloud.core.network.dto.AuthTokensDto
import com.mycloud.core.network.dto.UserDto
import java.time.Instant

fun UserDto.toDomain(): User = User(
    id = id,
    username = username,
    email = email,
    role = role.toUserRole(),
    quotaBytes = quotaBytes,
    usedBytes = usedBytes,
    isActive = isActive,
    mustChangePassword = mustChangePassword,
    createdAtMillis = createdAt.toEpochMillisOrZero(),
)

fun AuthTokensDto.toTokenBundle(): TokenBundle = TokenBundle(
    accessToken = accessToken,
    accessExpiresAtMillis = accessExpiresAt.toEpochMillisOrZero(),
    refreshToken = refreshToken,
    family = family,
    userId = userId,
)

/** Minimal user assembled from the login response (full profile via GET /me). */
fun AuthTokensDto.toUser(): User = User(
    id = userId,
    username = username,
    email = email,
    role = role.toUserRole(),
    quotaBytes = 0,
    usedBytes = 0,
    isActive = true,
    mustChangePassword = mustChangePassword,
    createdAtMillis = 0,
)

private fun String.toUserRole(): UserRole =
    if (equals("admin", ignoreCase = true)) UserRole.ADMIN else UserRole.USER

private fun String?.toEpochMillisOrZero(): Long =
    this?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() } ?: 0L
