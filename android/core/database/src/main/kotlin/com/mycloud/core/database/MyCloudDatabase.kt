package com.mycloud.core.database

import androidx.room.Database
import androidx.room.RoomDatabase
import com.mycloud.core.database.dao.FileDao
import com.mycloud.core.database.dao.FolderDao
import com.mycloud.core.database.dao.RemoteKeyDao
import com.mycloud.core.database.entity.FileEntity
import com.mycloud.core.database.entity.FolderEntity
import com.mycloud.core.database.entity.RemoteKeyEntity

@Database(
    entities = [FileEntity::class, FolderEntity::class, RemoteKeyEntity::class],
    version = 1,
    exportSchema = true,
)
abstract class MyCloudDatabase : RoomDatabase() {
    abstract fun fileDao(): FileDao
    abstract fun folderDao(): FolderDao
    abstract fun remoteKeyDao(): RemoteKeyDao
}
