package com.mycloud.core.database.entity

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Per-listing pagination cursor for the Paging `RemoteMediator`. The [queryKey]
 *  encodes folder + sort + order so distinct listings don't clobber each other. */
@Entity(tableName = "remote_keys")
data class RemoteKeyEntity(
    @PrimaryKey val queryKey: String,
    val nextCursor: String?,
)
