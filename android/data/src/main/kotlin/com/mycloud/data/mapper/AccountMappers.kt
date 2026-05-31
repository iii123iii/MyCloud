package com.mycloud.data.mapper

import com.mycloud.core.model.ActivityEntry
import com.mycloud.core.model.PersonalAccessToken
import com.mycloud.core.model.Session
import com.mycloud.core.network.dto.ActivityDto
import com.mycloud.core.network.dto.SessionDto
import com.mycloud.core.network.dto.TokenDto

fun SessionDto.toDomain(): Session = Session(
    jti = jti,
    deviceLabel = deviceLabel ?: userAgent,
    ipAddress = ipAddress,
    lastSeenAtMillis = lastSeenAt.isoToEpochMillis(),
    isCurrent = isCurrent,
)

fun ActivityDto.toDomain(): ActivityEntry = ActivityEntry(
    id = id.toString(),
    action = action,
    resourceType = resourceType,
    ipAddress = ipAddress,
    createdAtMillis = createdAt.isoToEpochMillis(),
)

fun TokenDto.toDomain(): PersonalAccessToken = PersonalAccessToken(
    id = id,
    name = name,
    tokenPrefix = tokenPrefix,
    lastFour = lastFour,
    scopes = scopes,
    createdAtMillis = createdAt.isoToEpochMillis(),
    expiresAtMillis = expiresAt?.isoToEpochMillis()?.takeIf { it != 0L },
)
