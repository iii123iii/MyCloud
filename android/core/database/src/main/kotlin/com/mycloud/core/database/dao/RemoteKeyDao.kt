package com.mycloud.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert
import com.mycloud.core.database.entity.RemoteKeyEntity

@Dao
interface RemoteKeyDao {

    @Upsert
    suspend fun upsert(key: RemoteKeyEntity)

    @Query("SELECT * FROM remote_keys WHERE queryKey = :queryKey")
    suspend fun get(queryKey: String): RemoteKeyEntity?

    @Query("DELETE FROM remote_keys WHERE queryKey = :queryKey")
    suspend fun delete(queryKey: String)
}
