@file:OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)

package com.mycloud.feature.browser

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.CreateNewFolder
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.model.Folder
import com.mycloud.data.repository.FolderRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import javax.inject.Inject

/** What the folder picker will do once a destination is chosen. */
enum class FolderPickerMode { MOVE, COPY }

@OptIn(ExperimentalCoroutinesApi::class)
@HiltViewModel
class FolderPickerViewModel @Inject constructor(
    private val folderRepository: FolderRepository,
) : ViewModel() {

    private val currentId = MutableStateFlow<String?>(null)
    val currentFolderId: StateFlow<String?> = currentId.asStateFlow()

    private val _title = MutableStateFlow("My Files")
    val title: StateFlow<String> = _title.asStateFlow()

    private val _isLoading = MutableStateFlow(false)
    /** True while the children of the current folder are being fetched. */
    val isLoading: StateFlow<Boolean> = _isLoading.asStateFlow()

    private val _isCreatingFolder = MutableStateFlow(false)
    val isCreatingFolder: StateFlow<Boolean> = _isCreatingFolder.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    // (id, title) of each ancestor so the nav-icon can walk back up.
    private val backStack = ArrayDeque<Pair<String?, String>>()

    val folders: StateFlow<List<Folder>> = currentId
        .flatMapLatest { folderRepository.folders(it) }
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), emptyList())

    init {
        load(null)
    }

    /**
     * Return the picker to its root state. This VM is reused across reopens, so
     * without an explicit reset the previous session's back stack/location would
     * leak into the next Move/Copy.
     */
    fun reset() {
        backStack.clear()
        currentId.value = null
        _title.value = "My Files"
        _error.value = null
        _isCreatingFolder.value = false
        load(null)
    }

    fun enter(folder: Folder) {
        _error.value = null
        backStack.addLast(currentId.value to _title.value)
        currentId.value = folder.id
        _title.value = folder.name
        load(folder.id)
    }

    /** @return true if a level was popped; false if already at the root. */
    fun up(): Boolean {
        val previous = backStack.removeLastOrNull() ?: return false
        _error.value = null
        currentId.value = previous.first
        _title.value = previous.second
        load(previous.first)
        return true
    }

    private fun load(folderId: String?) {
        _isLoading.value = true
        viewModelScope.launch {
            try {
                folderRepository.refreshFolders(folderId)
            } finally {
                _isLoading.value = false
            }
        }
    }

    fun clearError() {
        _error.value = null
    }

    fun createFolder(name: String, onResult: (Boolean) -> Unit = {}) {
        val trimmed = name.trim()
        if (trimmed.isEmpty()) {
            onResult(false)
            return
        }
        if (_isCreatingFolder.value) return
        _isCreatingFolder.value = true
        _error.value = null
        viewModelScope.launch {
            val result = folderRepository.createFolder(trimmed, currentId.value)
            _isCreatingFolder.value = false
            if (result is NetworkResult.Success) {
                onResult(true)
            } else {
                _error.value = result.userMessageOrNull() ?: "Couldn't create folder"
                onResult(false)
            }
        }
    }
}

/**
 * Full-screen destination chooser for Move/Copy. Tapping a folder descends into
 * it; the action targets whatever folder is currently shown ("… here"). Folders
 * being moved are shown disabled so you can't drop a folder inside itself.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FolderPicker(
    mode: FolderPickerMode,
    excludedFolderIds: Set<String>,
    onConfirm: (destFolderId: String?) -> Unit,
    onDismiss: () -> Unit,
    viewModel: FolderPickerViewModel = hiltViewModel(),
) {
    val folders by viewModel.folders.collectAsStateWithLifecycle()
    val location by viewModel.title.collectAsStateWithLifecycle()
    val currentId by viewModel.currentFolderId.collectAsStateWithLifecycle()
    val isLoading by viewModel.isLoading.collectAsStateWithLifecycle()
    val isCreatingFolder by viewModel.isCreatingFolder.collectAsStateWithLifecycle()
    val error by viewModel.error.collectAsStateWithLifecycle()
    var showNewFolder by remember { mutableStateOf(false) }

    // The VM survives reopens; reset to root each time the picker is shown.
    LaunchedEffect(Unit) { viewModel.reset() }

    val verb = if (mode == FolderPickerMode.MOVE) "Move" else "Copy"

    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        // Within a Dialog, system-back walks up the folder tree before dismissing.
        BackHandler { if (!viewModel.up()) onDismiss() }
        Surface(Modifier.fillMaxSize()) {
            Scaffold(
                topBar = {
                    TopAppBar(
                        title = {
                            Column {
                                Text("$verb to…", style = MaterialTheme.typography.titleMedium)
                                Text(
                                    location,
                                    style = MaterialTheme.typography.labelMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        },
                        navigationIcon = {
                            IconButton(onClick = { if (!viewModel.up()) onDismiss() }) {
                                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Up")
                            }
                        },
                        actions = {
                            IconButton(onClick = { showNewFolder = true }) {
                                Icon(Icons.Filled.CreateNewFolder, contentDescription = "New folder")
                            }
                        },
                    )
                },
                bottomBar = {
                    // Explicit container color — a bare tonalElevation is nearly
                    // invisible in dark mode, which read as "no background".
                    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh) {
                        Row(
                            Modifier
                                .fillMaxWidth()
                                .navigationBarsPadding()
                                .padding(horizontal = 16.dp, vertical = 12.dp),
                            horizontalArrangement = Arrangement.End,
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            TextButton(onClick = onDismiss) { Text("Cancel") }
                            Button(onClick = { onConfirm(currentId) }) { Text("$verb here") }
                        }
                    }
                },
            ) { padding ->
                if (isLoading && folders.isEmpty()) {
                    Box(
                        Modifier
                            .padding(padding)
                            .fillMaxSize(),
                        contentAlignment = Alignment.Center,
                    ) {
                        LoadingIndicator()
                    }
                } else if (folders.isEmpty()) {
                    Box(
                        Modifier
                            .padding(padding)
                            .fillMaxSize(),
                        contentAlignment = Alignment.Center,
                    ) {
                        Text(
                            "No subfolders here.",
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                } else {
                    LazyColumn(
                        Modifier
                            .padding(padding)
                            .fillMaxSize(),
                    ) {
                        items(folders, key = { it.id }) { folder ->
                            val excluded = folder.id in excludedFolderIds
                            ListItem(
                                headlineContent = {
                                    Text(
                                        folder.name,
                                        color = if (excluded) {
                                            MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f)
                                        } else {
                                            MaterialTheme.colorScheme.onSurface
                                        },
                                    )
                                },
                                supportingContent = if (excluded) {
                                    {
                                        Text(
                                            "Can't $verb a folder into itself",
                                            style = MaterialTheme.typography.bodySmall,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f),
                                        )
                                    }
                                } else {
                                    null
                                },
                                leadingContent = {
                                    Icon(
                                        Icons.Filled.Folder,
                                        contentDescription = null,
                                        tint = if (excluded) {
                                            MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f)
                                        } else {
                                            MaterialTheme.colorScheme.primary
                                        },
                                    )
                                },
                                modifier = if (excluded) {
                                    Modifier
                                } else {
                                    Modifier.clickable { viewModel.enter(folder) }
                                },
                            )
                        }
                    }
                }
            }
        }
    }

    if (showNewFolder) {
        var name by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = {
                if (!isCreatingFolder) {
                    viewModel.clearError()
                    showNewFolder = false
                }
            },
            title = { Text("New folder") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(
                        value = name,
                        onValueChange = {
                            name = it
                            viewModel.clearError()
                        },
                        singleLine = true,
                        label = { Text("Folder name") },
                        isError = error != null,
                    )
                    error?.let {
                        Text(
                            it,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        viewModel.createFolder(name) { ok -> if (ok) showNewFolder = false }
                    },
                    enabled = name.isNotBlank() && !isCreatingFolder,
                ) { Text(if (isCreatingFolder) "Creating..." else "Create") }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        viewModel.clearError()
                        showNewFolder = false
                    },
                    enabled = !isCreatingFolder,
                ) { Text("Cancel") }
            },
        )
    }
}
