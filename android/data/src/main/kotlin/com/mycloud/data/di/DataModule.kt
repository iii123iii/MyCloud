package com.mycloud.data.di

import com.mycloud.core.network.auth.TokenProvider
import com.mycloud.core.network.config.ServerUrlSource
import com.mycloud.data.auth.TokenProviderImpl
import com.mycloud.data.config.ServerUrlSourceImpl
import com.mycloud.data.repository.AccountRepository
import com.mycloud.data.repository.AccountRepositoryImpl
import com.mycloud.data.repository.AuthRepository
import com.mycloud.data.repository.AuthRepositoryImpl
import com.mycloud.data.repository.CommentRepository
import com.mycloud.data.repository.CommentRepositoryImpl
import com.mycloud.data.repository.FileRepository
import com.mycloud.data.repository.FileRepositoryImpl
import com.mycloud.data.repository.FolderRepository
import com.mycloud.data.repository.FolderRepositoryImpl
import com.mycloud.data.repository.PhotoRepository
import com.mycloud.data.repository.PhotoRepositoryImpl
import com.mycloud.data.repository.ShareRepository
import com.mycloud.data.repository.ShareRepositoryImpl
import com.mycloud.data.repository.StorageRepository
import com.mycloud.data.repository.StorageRepositoryImpl
import com.mycloud.data.repository.TransferRepository
import com.mycloud.data.repository.TransferRepositoryImpl
import com.mycloud.data.repository.TrashRepository
import com.mycloud.data.repository.TrashRepositoryImpl
import com.mycloud.data.repository.VersionRepository
import com.mycloud.data.repository.VersionRepositoryImpl
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
abstract class DataModule {

    @Binds
    @Singleton
    abstract fun bindAuthRepository(impl: AuthRepositoryImpl): AuthRepository

    @Binds
    @Singleton
    abstract fun bindTokenProvider(impl: TokenProviderImpl): TokenProvider

    @Binds
    @Singleton
    abstract fun bindServerUrlSource(impl: ServerUrlSourceImpl): ServerUrlSource

    @Binds
    @Singleton
    abstract fun bindFileRepository(impl: FileRepositoryImpl): FileRepository

    @Binds
    @Singleton
    abstract fun bindFolderRepository(impl: FolderRepositoryImpl): FolderRepository

    @Binds
    @Singleton
    abstract fun bindStorageRepository(impl: StorageRepositoryImpl): StorageRepository

    @Binds
    @Singleton
    abstract fun bindTransferRepository(impl: TransferRepositoryImpl): TransferRepository

    @Binds
    @Singleton
    abstract fun bindShareRepository(impl: ShareRepositoryImpl): ShareRepository

    @Binds
    @Singleton
    abstract fun bindTrashRepository(impl: TrashRepositoryImpl): TrashRepository

    @Binds
    @Singleton
    abstract fun bindPhotoRepository(impl: PhotoRepositoryImpl): PhotoRepository

    @Binds
    @Singleton
    abstract fun bindCommentRepository(impl: CommentRepositoryImpl): CommentRepository

    @Binds
    @Singleton
    abstract fun bindVersionRepository(impl: VersionRepositoryImpl): VersionRepository

    @Binds
    @Singleton
    abstract fun bindAccountRepository(impl: AccountRepositoryImpl): AccountRepository
}
