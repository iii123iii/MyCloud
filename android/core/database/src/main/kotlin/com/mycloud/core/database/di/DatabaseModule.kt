package com.mycloud.core.database.di

import android.content.Context
import androidx.room.Room
import com.mycloud.core.database.MyCloudDatabase
import com.mycloud.core.database.dao.FileDao
import com.mycloud.core.database.dao.FolderDao
import com.mycloud.core.database.dao.RemoteKeyDao
import com.mycloud.core.database.security.DbKeyProvider
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import net.zetetic.database.sqlcipher.SupportOpenHelperFactory
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object DatabaseModule {

    @Provides
    @Singleton
    fun provideDatabase(
        @ApplicationContext context: Context,
        dbKeyProvider: DbKeyProvider,
    ): MyCloudDatabase {
        // Load the SQLCipher native library before the encrypted DB opens.
        System.loadLibrary("sqlcipher")
        // A pre-existing plaintext cache from before encryption can't be opened with
        // a key, so we use a new filename and best-effort drop the old one. The cache
        // is disposable (server is source of truth), so this loses nothing.
        runCatching { context.deleteDatabase("mycloud.db") }
        return Room.databaseBuilder(context, MyCloudDatabase::class.java, "mycloud-enc.db")
            .openHelperFactory(SupportOpenHelperFactory(dbKeyProvider.passphrase()))
            // Phase 1: destructive fallback. Real migrations + tests land with the
            // schema as it stabilises (see plan's migration-testing note).
            .fallbackToDestructiveMigration()
            .build()
    }

    @Provides
    fun provideFileDao(db: MyCloudDatabase): FileDao = db.fileDao()

    @Provides
    fun provideFolderDao(db: MyCloudDatabase): FolderDao = db.folderDao()

    @Provides
    fun provideRemoteKeyDao(db: MyCloudDatabase): RemoteKeyDao = db.remoteKeyDao()
}
