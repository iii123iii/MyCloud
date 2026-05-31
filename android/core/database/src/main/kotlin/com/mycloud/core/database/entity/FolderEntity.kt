package com.mycloud.core.database.entity

import androidx.room.Entity
import androidx.room.PrimaryKey

@Entity(tableName = "folders")
data class FolderEntity(
    @PrimaryKey val id: String,
    val name: String,
    val parentId: String?,
    val createdAtMillis: Long,
    val updatedAtMillis: Long,
    val isFavorited: Boolean,
    val ownerId: String?,
    val permission: String?,
    val shared: Boolean,
)
