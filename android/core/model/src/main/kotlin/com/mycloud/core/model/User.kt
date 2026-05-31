package com.mycloud.core.model

/** The signed-in user, from `GET /auth/me`. Timestamps are epoch milliseconds. */
data class User(
    val id: String,
    val username: String,
    val email: String,
    val role: UserRole,
    val quotaBytes: Long,
    val usedBytes: Long,
    val isActive: Boolean,
    val mustChangePassword: Boolean,
    val createdAtMillis: Long,
)

/** Aggregate storage figures from `GET /storage/stats`. */
data class StorageStats(
    val quotaBytes: Long,
    val usedBytes: Long,
    val availableBytes: Long,
    val fileCount: Long,
    val folderCount: Long,
)
