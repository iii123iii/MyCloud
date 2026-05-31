package com.mycloud.feature.browser

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Audiotrack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.CreateNewFolder
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.DriveFileMove
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.SwapVert
import androidx.compose.material.icons.filled.Upload
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.PictureAsPdf
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.Sort
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.StarBorder
import androidx.compose.material.icons.filled.ViewList
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Card
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch
import androidx.paging.LoadState
import androidx.paging.compose.collectAsLazyPagingItems
import androidx.paging.compose.itemKey
import coil3.compose.AsyncImage
import com.mycloud.core.common.util.ByteFormatter
import com.mycloud.core.common.util.FileKind
import com.mycloud.core.common.util.MimeType
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.Folder
import com.mycloud.core.model.SortKey
import com.mycloud.core.model.SortOrder
import com.mycloud.core.model.Transfer
import com.mycloud.core.model.TransferDirection
import com.mycloud.core.model.TransferStatus
import com.mycloud.core.model.ViewMode

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun BrowserScreen(
    modifier: Modifier = Modifier,
    viewModel: BrowserViewModel = hiltViewModel(),
    onOpenFile: (FileNode) -> Unit = {},
    onShareFile: (FileNode) -> Unit = {},
    onShareFolder: (Folder) -> Unit = {},
) {
    val ui by viewModel.uiState.collectAsStateWithLifecycle()
    val files = viewModel.files.collectAsLazyPagingItems()
    val transfers by viewModel.transfers.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var showNewFolder by remember { mutableStateOf(false) }
    var showFabMenu by remember { mutableStateOf(false) }
    var showTransfers by remember { mutableStateOf(false) }
    var renameTarget by remember { mutableStateOf<RenameTarget?>(null) }
    var pickerMode by remember { mutableStateOf<FolderPickerMode?>(null) }
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()

    val uploadLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenMultipleDocuments(),
    ) { uris ->
        uris.forEach { uri ->
            runCatching {
                context.contentResolver.takePersistableUriPermission(
                    uri,
                    Intent.FLAG_GRANT_READ_URI_PERMISSION,
                )
            }
            viewModel.upload(uri.toString(), context.queryDisplayName(uri))
        }
        if (uris.isNotEmpty()) {
            scope.launch {
                snackbarHostState.showSnackbar("Uploading ${uris.size} file(s)…")
            }
        }
    }

    val selectedFiles = files.itemSnapshotList.items.filter { it.id in ui.selectedIds }
    val selectedFolders = ui.folders.filter { it.id in ui.selectedFolderIds }
    val singleSelection = ui.selectionCount == 1
    val allSelectedStarred = selectedFiles.isNotEmpty() &&
        selectedFolders.isEmpty() &&
        selectedFiles.all { it.isStarred }

    fun shareSelection() {
        selectedFiles.singleOrNull()?.let(onShareFile)
            ?: selectedFolders.singleOrNull()?.let(onShareFolder)
    }

    fun renameSelection() {
        selectedFiles.singleOrNull()?.let { renameTarget = RenameTarget(it.id, it.name, false) }
            ?: selectedFolders.singleOrNull()?.let { renameTarget = RenameTarget(it.id, it.name, true) }
    }

    BackHandler(enabled = ui.canGoBack || ui.inSelectionMode) {
        if (ui.inSelectionMode) viewModel.clearSelection() else viewModel.onBack()
    }

    LaunchedEffect(Unit) {
        viewModel.refreshSignals.collect { files.refresh() }
    }

    // Auto-fetch when the browser (re)appears — e.g. right after signing in — so the
    // list is never stale/empty waiting for a manual pull-to-refresh.
    LaunchedEffect(Unit) {
        files.refresh()
        viewModel.refresh()
    }

    LaunchedEffect(Unit) {
        viewModel.messages.collect { snackbarHostState.showSnackbar(it) }
    }

    Scaffold(
        modifier = modifier,
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            if (ui.inSelectionMode) {
                SelectionTopBar(
                    count = ui.selectionCount,
                    shareEnabled = singleSelection,
                    renameEnabled = singleSelection,
                    starred = allSelectedStarred,
                    onClose = viewModel::clearSelection,
                    onShare = ::shareSelection,
                    onRename = ::renameSelection,
                    onDownload = { viewModel.downloadSelection(selectedFiles, selectedFolders) },
                    onStar = { viewModel.starSelected(!allSelectedStarred) },
                    onMove = { pickerMode = FolderPickerMode.MOVE },
                    onCopy = { pickerMode = FolderPickerMode.COPY },
                    onDelete = viewModel::deleteSelected,
                )
            } else {
                BrowserTopBar(
                    title = ui.title,
                    canGoBack = ui.canGoBack,
                    viewMode = ui.viewMode,
                    transferCount = transfers.count { it.status == TransferStatus.RUNNING || it.status == TransferStatus.QUEUED },
                    onTransfers = { showTransfers = true },
                    onBack = { viewModel.onBack() },
                    onToggleView = {
                        viewModel.setViewMode(
                            if (ui.viewMode == ViewMode.GRID) ViewMode.LIST else ViewMode.GRID,
                        )
                    },
                    onSort = viewModel::setSort,
                )
            }
        },
        floatingActionButton = {
            if (!ui.inSelectionMode) Box {
                FloatingActionButton(onClick = { showFabMenu = true }) {
                    Icon(Icons.Filled.Add, contentDescription = "Add")
                }
                DropdownMenu(expanded = showFabMenu, onDismissRequest = { showFabMenu = false }) {
                    DropdownMenuItem(
                        text = { Text("Upload files") },
                        leadingIcon = { Icon(Icons.Filled.Upload, contentDescription = null) },
                        onClick = {
                            showFabMenu = false
                            uploadLauncher.launch(arrayOf("*/*"))
                        },
                    )
                    DropdownMenuItem(
                        text = { Text("New folder") },
                        leadingIcon = { Icon(Icons.Filled.CreateNewFolder, contentDescription = null) },
                        onClick = {
                            showFabMenu = false
                            showNewFolder = true
                        },
                    )
                }
            }
        },
    ) { padding ->
        Box(
            Modifier
                .padding(padding)
                .fillMaxSize(),
        ) {
            Column(Modifier.fillMaxSize()) {
                if (ui.breadcrumbs.isNotEmpty()) {
                    BreadcrumbRow(ui)
                }
                PullToRefreshBox(
                    isRefreshing = files.loadState.refresh is LoadState.Loading,
                    onRefresh = {
                        files.refresh()
                        viewModel.refresh()
                    },
                ) {
                    if (ui.viewMode == ViewMode.GRID) {
                        FileGrid(ui, files, viewModel, onOpenFile)
                    } else {
                        FileList(ui, files, viewModel, onOpenFile)
                    }
                }
            }
        }
    }

    if (showNewFolder) {
        NewFolderDialog(
            onConfirm = {
                viewModel.createFolder(it)
                showNewFolder = false
            },
            onDismiss = { showNewFolder = false },
        )
    }

    if (showTransfers) {
        TransfersSheet(transfers = transfers, onDismiss = { showTransfers = false })
    }

    renameTarget?.let { target ->
        RenameDialog(
            initial = target.name,
            onConfirm = { newName ->
                viewModel.rename(target.id, target.isFolder, newName)
                renameTarget = null
            },
            onDismiss = { renameTarget = null },
        )
    }

    pickerMode?.let { mode ->
        FolderPicker(
            mode = mode,
            // Can't move a folder into itself; greyed out in the picker.
            excludedFolderIds = ui.selectedFolderIds,
            onConfirm = { destFolderId ->
                when (mode) {
                    FolderPickerMode.MOVE -> viewModel.moveSelected(destFolderId)
                    FolderPickerMode.COPY -> viewModel.copySelected(destFolderId)
                }
                pickerMode = null
            },
            onDismiss = { pickerMode = null },
        )
    }
}

private data class RenameTarget(val id: String, val name: String, val isFolder: Boolean)

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun FileGrid(
    ui: BrowserUiState,
    files: androidx.paging.compose.LazyPagingItems<FileNode>,
    viewModel: BrowserViewModel,
    onOpenFile: (FileNode) -> Unit,
) {
    LazyVerticalGrid(
        columns = GridCells.Adaptive(minSize = 112.dp),
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(12.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        items(ui.folders, key = { "folder:${it.id}" }) { folder ->
            FolderGridCard(
                folder = folder,
                selected = folder.id in ui.selectedFolderIds,
                onClick = {
                    if (ui.inSelectionMode) viewModel.toggleFolderSelect(folder.id) else viewModel.openFolder(folder)
                },
                onLongClick = { viewModel.toggleFolderSelect(folder.id) },
                modifier = Modifier.animateItem(),
            )
        }
        items(count = files.itemCount, key = files.itemKey { it.id }) { index ->
            val file = files[index] ?: return@items
            FileGridCard(
                file = file,
                thumbnailUrl = viewModel.thumbnailUrl(file.id),
                selected = file.id in ui.selectedIds,
                onClick = {
                    if (ui.inSelectionMode) viewModel.toggleSelect(file.id) else onOpenFile(file)
                },
                onLongClick = { viewModel.toggleSelect(file.id) },
                modifier = Modifier.animateItem(),
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
private fun FileList(
    ui: BrowserUiState,
    files: androidx.paging.compose.LazyPagingItems<FileNode>,
    viewModel: BrowserViewModel,
    onOpenFile: (FileNode) -> Unit,
) {
    LazyColumn(Modifier.fillMaxSize()) {
        items(ui.folders, key = { "folder:${it.id}" }) { folder ->
            val selected = folder.id in ui.selectedFolderIds
            ListItem(
                headlineContent = { Text(folder.name) },
                leadingContent = { Icon(Icons.Filled.Folder, contentDescription = null) },
                colors = if (selected) {
                    ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.secondaryContainer)
                } else {
                    ListItemDefaults.colors()
                },
                modifier = Modifier
                    .animateItem()
                    .combinedClickable(
                        onClick = {
                            if (ui.inSelectionMode) viewModel.toggleFolderSelect(folder.id) else viewModel.openFolder(folder)
                        },
                        onLongClick = { viewModel.toggleFolderSelect(folder.id) },
                    ),
            )
        }
        items(count = files.itemCount, key = files.itemKey { it.id }) { index ->
            val file = files[index] ?: return@items
            FileListRow(
                file = file,
                selected = file.id in ui.selectedIds,
                onClick = {
                    if (ui.inSelectionMode) viewModel.toggleSelect(file.id) else onOpenFile(file)
                },
                onLongClick = { viewModel.toggleSelect(file.id) },
                modifier = Modifier.animateItem(),
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BrowserTopBar(
    title: String,
    canGoBack: Boolean,
    viewMode: ViewMode,
    transferCount: Int,
    onTransfers: () -> Unit,
    onBack: () -> Unit,
    onToggleView: () -> Unit,
    onSort: (SortKey, SortOrder) -> Unit,
) {
    var sortOpen by remember { mutableStateOf(false) }
    TopAppBar(
        title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        navigationIcon = {
            if (canGoBack) {
                IconButton(onClick = onBack) {
                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                }
            }
        },
        actions = {
            IconButton(onClick = onTransfers) {
                BadgedBox(
                    badge = {
                        if (transferCount > 0) Badge { Text("$transferCount") }
                    },
                ) {
                    Icon(
                        Icons.Filled.SwapVert,
                        contentDescription = if (transferCount > 0) "$transferCount active transfers" else "Transfers",
                    )
                }
            }
            IconButton(onClick = onToggleView) {
                Icon(
                    if (viewMode == ViewMode.GRID) Icons.Filled.ViewList else Icons.Filled.GridView,
                    contentDescription = "Toggle view",
                )
            }
            IconButton(onClick = { sortOpen = true }) {
                Icon(Icons.Filled.Sort, contentDescription = "Sort")
            }
            DropdownMenu(expanded = sortOpen, onDismissRequest = { sortOpen = false }) {
                DropdownMenuItem(
                    text = { Text("Name (A–Z)") },
                    onClick = { onSort(SortKey.NAME, SortOrder.ASC); sortOpen = false },
                )
                DropdownMenuItem(
                    text = { Text("Newest") },
                    onClick = { onSort(SortKey.UPDATED, SortOrder.DESC); sortOpen = false },
                )
                DropdownMenuItem(
                    text = { Text("Largest") },
                    onClick = { onSort(SortKey.SIZE, SortOrder.DESC); sortOpen = false },
                )
            }
        },
    )
}

@Composable
private fun BreadcrumbRow(ui: BrowserUiState) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = "My Files / " + ui.breadcrumbs.joinToString(" / ") { it.name },
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun FolderGridCard(
    folder: Folder,
    selected: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val border = if (selected) BorderStroke(2.dp, MaterialTheme.colorScheme.primary) else null
    // Mirror FileGridCard's structure (same thumbnail aspect + name row) so folder
    // and file tiles are identical heights and the grid has no uneven gaps.
    Card(
        modifier = modifier.combinedClickable(onClick = onClick, onLongClick = onLongClick),
        border = border,
    ) {
        Column {
            Box(
                Modifier
                    .fillMaxWidth()
                    .aspectRatio(1.2f),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    Icons.Filled.Folder,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(40.dp),
                )
            }
            Text(
                folder.name,
                style = MaterialTheme.typography.bodySmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(horizontal = 8.dp, vertical = 6.dp),
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun FileGridCard(
    file: FileNode,
    thumbnailUrl: String,
    selected: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val border = if (selected) {
        BorderStroke(2.dp, MaterialTheme.colorScheme.primary)
    } else {
        null
    }
    Card(
        modifier = modifier.combinedClickable(onClick = onClick, onLongClick = onLongClick),
        border = border,
    ) {
        Column {
            Box(
                Modifier
                    .fillMaxWidth()
                    .aspectRatio(1.2f)
                    .clip(RoundedCornerShape(0.dp)),
                contentAlignment = Alignment.Center,
            ) {
                if (MimeType.kindOf(file.mimeType) == FileKind.IMAGE) {
                    AsyncImage(
                        model = thumbnailUrl,
                        contentDescription = file.name,
                        modifier = Modifier.fillMaxSize(),
                    )
                } else {
                    Icon(
                        imageVector = fileKindIcon(MimeType.kindOf(file.mimeType)),
                        contentDescription = null,
                        modifier = Modifier.size(40.dp),
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (file.isStarred) {
                    Icon(
                        Icons.Filled.Star,
                        contentDescription = "Starred",
                        tint = MaterialTheme.colorScheme.primary,
                        modifier = Modifier
                            .align(Alignment.TopEnd)
                            .padding(4.dp)
                            .size(18.dp),
                    )
                }
            }
            Text(
                file.name,
                style = MaterialTheme.typography.bodySmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(horizontal = 8.dp, vertical = 6.dp),
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class, ExperimentalMaterial3Api::class)
@Composable
private fun FileListRow(
    file: FileNode,
    selected: Boolean,
    onClick: () -> Unit,
    onLongClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    ListItem(
        headlineContent = { Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        supportingContent = { Text(ByteFormatter.format(file.sizeBytes)) },
        leadingContent = {
            Icon(fileKindIcon(MimeType.kindOf(file.mimeType)), contentDescription = null)
        },
        trailingContent = {
            if (file.isStarred) {
                Icon(
                    Icons.Filled.Star,
                    contentDescription = "Starred",
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
        },
        colors = if (selected) {
            ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.secondaryContainer)
        } else {
            ListItemDefaults.colors()
        },
        modifier = modifier.combinedClickable(onClick = onClick, onLongClick = onLongClick),
    )
}

/**
 * The Android-idiomatic multi-select bar: a contextual top app bar that replaces
 * the normal one while items are selected — a Close action, the selected count,
 * and the available operations.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SelectionTopBar(
    count: Int,
    shareEnabled: Boolean,
    renameEnabled: Boolean,
    starred: Boolean,
    onClose: () -> Unit,
    onShare: () -> Unit,
    onRename: () -> Unit,
    onDownload: () -> Unit,
    onStar: () -> Unit,
    onMove: () -> Unit,
    onCopy: () -> Unit,
    onDelete: () -> Unit,
) {
    var overflowOpen by remember { mutableStateOf(false) }
    TopAppBar(
        title = { Text("$count selected") },
        navigationIcon = {
            IconButton(onClick = onClose) { Icon(Icons.Filled.Close, contentDescription = "Clear selection") }
        },
        // Keep the bar uncramped: the three most-common actions stay as icons; the
        // rest live in the overflow menu.
        actions = {
            IconButton(onClick = onShare, enabled = shareEnabled) {
                Icon(Icons.Filled.Share, contentDescription = "Share")
            }
            IconButton(onClick = onDownload) { Icon(Icons.Filled.Download, contentDescription = "Download") }
            IconButton(onClick = onDelete) { Icon(Icons.Filled.Delete, contentDescription = "Delete") }
            IconButton(onClick = { overflowOpen = true }) {
                Icon(Icons.Filled.MoreVert, contentDescription = "More")
            }
            DropdownMenu(expanded = overflowOpen, onDismissRequest = { overflowOpen = false }) {
                if (renameEnabled) {
                    DropdownMenuItem(
                        text = { Text("Rename") },
                        leadingIcon = { Icon(Icons.Filled.Edit, contentDescription = null) },
                        onClick = {
                            overflowOpen = false
                            onRename()
                        },
                    )
                }
                DropdownMenuItem(
                    text = { Text(if (starred) "Unstar" else "Star") },
                    leadingIcon = {
                        Icon(
                            if (starred) Icons.Filled.Star else Icons.Filled.StarBorder,
                            contentDescription = null,
                        )
                    },
                    onClick = {
                        overflowOpen = false
                        onStar()
                    },
                )
                DropdownMenuItem(
                    text = { Text("Move") },
                    leadingIcon = { Icon(Icons.Filled.DriveFileMove, contentDescription = null) },
                    onClick = {
                        overflowOpen = false
                        onMove()
                    },
                )
                DropdownMenuItem(
                    text = { Text("Copy") },
                    leadingIcon = { Icon(Icons.Filled.ContentCopy, contentDescription = null) },
                    onClick = {
                        overflowOpen = false
                        onCopy()
                    },
                )
            }
        },
        colors = TopAppBarDefaults.topAppBarColors(
            containerColor = MaterialTheme.colorScheme.secondaryContainer,
        ),
    )
}

@Composable
private fun NewFolderDialog(onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var name by remember { mutableStateOf("") }
    androidx.compose.material3.AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("New folder") },
        text = {
            androidx.compose.material3.OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                singleLine = true,
                label = { Text("Folder name") },
            )
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(name) }, enabled = name.isNotBlank()) { Text("Create") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun RenameDialog(initial: String, onConfirm: (String) -> Unit, onDismiss: () -> Unit) {
    var name by remember { mutableStateOf(initial) }
    androidx.compose.material3.AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Rename") },
        text = {
            androidx.compose.material3.OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                singleLine = true,
                label = { Text("Name") },
            )
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(name) }, enabled = name.isNotBlank()) { Text("Rename") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

private fun fileKindIcon(kind: FileKind): ImageVector = when (kind) {
    FileKind.IMAGE -> Icons.Filled.Image
    FileKind.VIDEO -> Icons.Filled.Movie
    FileKind.AUDIO -> Icons.Filled.Audiotrack
    FileKind.PDF -> Icons.Filled.PictureAsPdf
    else -> Icons.Filled.InsertDriveFile
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TransfersSheet(transfers: List<Transfer>, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp),
        ) {
            Text("Transfers", style = MaterialTheme.typography.titleLarge)
            Spacer(Modifier.height(12.dp))
            if (transfers.isEmpty()) {
                Text(
                    "No transfers yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                transfers.forEach { TransferRow(it) }
            }
            Spacer(Modifier.height(24.dp))
        }
    }
}

@Composable
private fun TransferRow(transfer: Transfer) {
    Column(
        Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(
                imageVector = if (transfer.direction == TransferDirection.UPLOAD) {
                    Icons.Filled.Upload
                } else {
                    Icons.Filled.Download
                },
                contentDescription = null,
            )
            Spacer(Modifier.size(8.dp))
            Text(
                transfer.name,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Text(transfer.statusLabel(), style = MaterialTheme.typography.labelMedium)
        }
        if (transfer.status == TransferStatus.RUNNING) {
            Spacer(Modifier.height(4.dp))
            LinearProgressIndicator(
                progress = { transfer.progress / 100f },
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

private fun Transfer.statusLabel(): String = when (status) {
    TransferStatus.QUEUED -> "Queued"
    TransferStatus.RUNNING -> "$progress%"
    TransferStatus.SUCCEEDED -> "Done"
    TransferStatus.FAILED -> "Failed"
}

private fun Context.queryDisplayName(uri: Uri): String {
    val name = runCatching {
        contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor ->
            val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (index >= 0 && cursor.moveToFirst()) cursor.getString(index) else null
        }
    }.getOrNull()
    return name ?: uri.lastPathSegment ?: "upload"
}
