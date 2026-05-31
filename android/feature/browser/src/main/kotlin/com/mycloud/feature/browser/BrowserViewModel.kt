package com.mycloud.feature.browser

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.paging.PagingData
import androidx.paging.cachedIn
import com.mycloud.core.model.Breadcrumb
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.Folder
import com.mycloud.core.model.SortKey
import com.mycloud.core.model.SortOrder
import com.mycloud.core.model.Transfer
import com.mycloud.core.model.TransferDirection
import com.mycloud.core.model.TransferStatus
import com.mycloud.core.model.ViewMode
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.data.realtime.RealtimeRepository
import com.mycloud.data.repository.AuthRepository
import com.mycloud.data.repository.FileRepository
import com.mycloud.data.repository.FolderRepository
import com.mycloud.data.repository.TransferRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class BrowserArgs(
    val folderId: String? = null,
    val sort: SortKey = SortKey.NAME,
    val order: SortOrder = SortOrder.ASC,
)

data class BrowserUiState(
    val folderId: String? = null,
    val title: String = "My Files",
    val canGoBack: Boolean = false,
    val breadcrumbs: List<Breadcrumb> = emptyList(),
    val folders: List<Folder> = emptyList(),
    val sort: SortKey = SortKey.NAME,
    val order: SortOrder = SortOrder.ASC,
    val viewMode: ViewMode = ViewMode.GRID,
    val selectedIds: Set<String> = emptySet(),
    val selectedFolderIds: Set<String> = emptySet(),
) {
    val inSelectionMode: Boolean get() = selectedIds.isNotEmpty() || selectedFolderIds.isNotEmpty()
    val selectionCount: Int get() = selectedIds.size + selectedFolderIds.size
}

@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
@HiltViewModel
class BrowserViewModel @Inject constructor(
    private val fileRepository: FileRepository,
    private val folderRepository: FolderRepository,
    private val transferRepository: TransferRepository,
    private val authRepository: AuthRepository,
    realtimeRepository: RealtimeRepository,
) : ViewModel() {

    private val args = MutableStateFlow(BrowserArgs())
    private val _ui = MutableStateFlow(BrowserUiState())
    val uiState: StateFlow<BrowserUiState> = _ui.asStateFlow()

    val transfers: StateFlow<List<Transfer>> = transferRepository.transfers()
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptyList())

    private val localRefresh = MutableSharedFlow<Unit>(extraBufferCapacity = 8)

    /** Emits on a realtime change OR when uploads finish; the screen refreshes paging. */
    val refreshSignals: Flow<Unit> = merge(realtimeRepository.fileEvents, localRefresh)

    private val _messages = MutableSharedFlow<String>(extraBufferCapacity = 4)

    /** Transient user-facing messages (e.g. a move/copy failure) shown as a snackbar. */
    val messages: SharedFlow<String> = _messages.asSharedFlow()

    /** Paged files for the active folder + sort. Cached so config changes don't refetch. */
    val files: Flow<PagingData<FileNode>> = args
        .flatMapLatest { fileRepository.pagedFiles(it.folderId, it.sort, it.order) }
        .cachedIn(viewModelScope)

    private data class Crumb(val id: String?, val title: String)
    private val backStack = ArrayDeque<Crumb>()

    init {
        // Mirror the cached folder list for the active parent into UI state.
        viewModelScope.launch {
            args.map { it.folderId }
                .distinctUntilChanged()
                .flatMapLatest { folderRepository.folders(it) }
                .collect { folders -> _ui.update { it.copy(folders = folders) } }
        }
        // When uploads finish, signal a refresh (active uploads → none).
        viewModelScope.launch {
            var previousActive = 0
            transfers.collect { list ->
                val active = list.count {
                    it.direction == TransferDirection.UPLOAD &&
                        (it.status == TransferStatus.RUNNING || it.status == TransferStatus.QUEUED)
                }
                if (active == 0 && previousActive > 0) localRefresh.tryEmit(Unit)
                previousActive = active
            }
        }
        // Any refresh signal (realtime or upload-complete) reloads cached subfolders.
        viewModelScope.launch {
            refreshSignals.collect { folderRepository.refreshFolders(_ui.value.folderId) }
        }
        // This VM is Activity-scoped and survives sign-out, so its one-time load
        // wouldn't re-run on re-login. Reset to root + reload whenever the signed-in
        // user changes (fresh login or user switch) — no manual pull-to-refresh needed.
        viewModelScope.launch {
            authRepository.currentUser
                .map { it?.id }
                .distinctUntilChanged()
                .collect { userId ->
                    if (userId != null) {
                        backStack.clear()
                        navigateTo(folderId = null, title = "My Files")
                        localRefresh.tryEmit(Unit)
                    }
                }
        }
    }

    fun openFolder(folder: Folder) {
        backStack.addLast(Crumb(_ui.value.folderId, _ui.value.title))
        navigateTo(folder.id, folder.name)
    }

    /** @return true if a level was popped; false if already at the root. */
    fun onBack(): Boolean {
        val previous = backStack.removeLastOrNull() ?: return false
        navigateTo(previous.id, previous.title)
        return true
    }

    private fun navigateTo(folderId: String?, title: String) {
        _ui.update {
            it.copy(
                folderId = folderId,
                title = title,
                canGoBack = backStack.isNotEmpty(),
                selectedIds = emptySet(),
                selectedFolderIds = emptySet(),
            )
        }
        args.update { it.copy(folderId = folderId) }
        viewModelScope.launch {
            folderRepository.refreshFolders(folderId)
            val crumbs = if (folderId == null) {
                emptyList()
            } else {
                (folderRepository.breadcrumb(folderId) as? NetworkResult.Success)?.data.orEmpty()
            }
            _ui.update { it.copy(breadcrumbs = crumbs) }
        }
    }

    fun refresh() {
        viewModelScope.launch { folderRepository.refreshFolders(_ui.value.folderId) }
    }

    fun setSort(sort: SortKey, order: SortOrder) {
        _ui.update { it.copy(sort = sort, order = order) }
        args.update { it.copy(sort = sort, order = order) }
    }

    fun setViewMode(mode: ViewMode) = _ui.update { it.copy(viewMode = mode) }

    fun toggleSelect(fileId: String) = _ui.update {
        val next = if (fileId in it.selectedIds) it.selectedIds - fileId else it.selectedIds + fileId
        it.copy(selectedIds = next)
    }

    fun toggleFolderSelect(folderId: String) = _ui.update {
        val next = if (folderId in it.selectedFolderIds) it.selectedFolderIds - folderId else it.selectedFolderIds + folderId
        it.copy(selectedFolderIds = next)
    }

    fun clearSelection() = _ui.update { it.copy(selectedIds = emptySet(), selectedFolderIds = emptySet()) }

    fun deleteSelected() {
        val fileIds = _ui.value.selectedIds
        val folderIds = _ui.value.selectedFolderIds
        val parent = _ui.value.folderId
        clearSelection()
        viewModelScope.launch {
            fileIds.forEach { fileRepository.deleteFile(it) }
            folderIds.forEach { folderRepository.deleteFolder(it) }
            if (folderIds.isNotEmpty()) folderRepository.refreshFolders(parent)
        }
    }

    fun starSelected(starred: Boolean) {
        val ids = _ui.value.selectedIds
        clearSelection()
        viewModelScope.launch { ids.forEach { fileRepository.setStarred(it, starred) } }
    }

    /** Rename a single selected file or folder. */
    fun rename(id: String, isFolder: Boolean, newName: String) {
        val trimmed = newName.trim()
        if (trimmed.isEmpty()) return
        val parent = _ui.value.folderId
        clearSelection()
        viewModelScope.launch {
            if (isFolder) {
                folderRepository.renameFolder(id, trimmed)
                folderRepository.refreshFolders(parent)
            } else {
                fileRepository.rename(id, trimmed)
            }
        }
    }

    /** Move every selected file + folder into [destFolderId] (null = root). */
    fun moveSelected(destFolderId: String?) {
        val fileIds = _ui.value.selectedIds
        val folderIds = _ui.value.selectedFolderIds
        val parent = _ui.value.folderId
        clearSelection()
        viewModelScope.launch {
            val results = fileIds.map { fileRepository.moveFile(it, destFolderId) } +
                folderIds.map { folderRepository.moveFolder(it, destFolderId) }
            // Re-sync source (lost items) and destination (gained items).
            folderRepository.refreshFolders(parent)
            if (destFolderId != parent) folderRepository.refreshFolders(destFolderId)
            localRefresh.tryEmit(Unit)
            reportFailures(results, "move")
        }
    }

    /** Copy every selected file + folder into [destFolderId] (null = root). */
    fun copySelected(destFolderId: String?) {
        val fileIds = _ui.value.selectedIds
        val folderIds = _ui.value.selectedFolderIds
        val parent = _ui.value.folderId
        clearSelection()
        viewModelScope.launch {
            val results = fileIds.map { fileRepository.copyFile(it, destFolderId) } +
                folderIds.map { folderRepository.copyFolder(it, destFolderId) }
            // The copies appear in the destination; refresh it (and the file list if it's here).
            folderRepository.refreshFolders(destFolderId)
            if (destFolderId == parent) localRefresh.tryEmit(Unit)
            reportFailures(results, "copy")
        }
    }

    /** Surface a snackbar if any of [results] failed (move/copy were silent before). */
    private fun reportFailures(results: List<NetworkResult<Unit>>, verb: String) {
        val failures = results.filter { it !is NetworkResult.Success }
        if (failures.isEmpty()) return
        val reason = failures.firstNotNullOfOrNull { it.userMessageOrNull() }
        _messages.tryEmit(reason ?: "Couldn't $verb ${failures.size} item(s)")
    }

    fun createFolder(name: String) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) return
        viewModelScope.launch { folderRepository.createFolder(trimmed, _ui.value.folderId) }
    }

    fun upload(uri: String, displayName: String) {
        transferRepository.uploadFile(uri, displayName, _ui.value.folderId)
    }

    /**
     * Download the current selection. Folders can't be fetched one-by-one, so any
     * selection that includes a folder is zipped server-side into a single archive;
     * a files-only selection keeps the per-file downloads (real filenames, parallel).
     */
    fun downloadSelection(files: List<FileNode>, folders: List<Folder>) {
        clearSelection()
        if (folders.isEmpty()) {
            files.forEach { transferRepository.download(it.id, it.name) }
        } else {
            val fileIds = files.map { it.id }
            val folderIds = folders.map { it.id }
            // Enqueuing the archive writes a small request file — keep it off the main thread.
            viewModelScope.launch(Dispatchers.IO) {
                transferRepository.downloadArchive(fileIds, folderIds, "mycloud.zip")
            }
        }
    }

    fun thumbnailUrl(fileId: String): String = fileRepository.thumbnailUrl(fileId)
}
