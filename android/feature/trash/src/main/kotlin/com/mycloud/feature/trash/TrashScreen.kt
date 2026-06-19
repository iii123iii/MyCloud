package com.mycloud.feature.trash

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.DeleteForever
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.Restore
import androidx.compose.material.icons.filled.SelectAll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.common.util.ByteFormatter
import com.mycloud.core.model.TrashItem
import kotlin.math.max

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TrashScreen(
    modifier: Modifier = Modifier,
    scrollToTopSignal: Int = 0,
    viewModel: TrashViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val listState = rememberLazyListState()

    // Auto-refresh when the screen (re)appears (the VM is Activity-scoped).
    LaunchedEffect(Unit) { viewModel.refresh() }
    // Re-tapping the Trash tab scrolls back to the top.
    LaunchedEffect(scrollToTopSignal) {
        if (scrollToTopSignal > 0) listState.animateScrollToItem(0)
    }

    // Confirmation targets: null = no dialog showing.
    var deleteTarget by remember { mutableStateOf<TrashItem?>(null) }
    var confirmEmpty by remember { mutableStateOf(false) }
    var confirmDeleteSelected by remember { mutableStateOf(false) }

    // Leaving selection mode (e.g. the list emptied out) shouldn't strand a stale dialog.
    LaunchedEffect(state.selectionMode) {
        if (!state.selectionMode) confirmDeleteSelected = false
    }

    androidx.compose.material3.Scaffold(
        modifier = modifier,
        topBar = {
            if (state.selectionMode) {
                TopAppBar(
                    title = { Text("${state.selectedIds.size} selected") },
                    navigationIcon = {
                        IconButton(onClick = viewModel::exitSelection) {
                            Icon(Icons.Filled.Close, contentDescription = "Exit selection")
                        }
                    },
                    actions = {
                        IconButton(
                            onClick = viewModel::selectAll,
                            enabled = !state.isLoading,
                        ) {
                            Icon(Icons.Filled.SelectAll, contentDescription = "Select all")
                        }
                        TextButton(
                            onClick = viewModel::restoreSelected,
                            enabled = !state.isLoading && state.selectedIds.isNotEmpty(),
                        ) { Text("Restore selected") }
                        TextButton(
                            onClick = { confirmDeleteSelected = true },
                            enabled = !state.isLoading && state.selectedIds.isNotEmpty(),
                        ) { Text("Delete selected forever") }
                    },
                )
            } else {
                TopAppBar(
                    title = { Text("Trash") },
                    actions = {
                        if (state.items.isNotEmpty()) {
                            TextButton(
                                onClick = viewModel::restoreAll,
                                enabled = !state.isLoading,
                            ) { Text("Restore all") }
                            TextButton(
                                onClick = { confirmEmpty = true },
                                enabled = !state.isLoading,
                            ) { Text("Empty") }
                        }
                    },
                )
            }
        },
    ) { padding ->
        PullToRefreshBox(
            isRefreshing = state.isLoading,
            onRefresh = viewModel::refresh,
            modifier = Modifier.fillMaxSize().padding(padding),
        ) {
            // Always a LazyColumn (even when empty) so the surrounding
            // PullToRefreshBox still gets the pull gesture via nested scroll.
            LazyColumn(modifier = Modifier.fillMaxSize(), state = listState) {
                if (state.items.isEmpty()) {
                    item {
                        Box(Modifier.fillParentMaxSize(), contentAlignment = Alignment.Center) {
                            Text(
                                state.error ?: "Trash is empty.",
                                color = if (state.error != null) {
                                    MaterialTheme.colorScheme.error
                                } else {
                                    MaterialTheme.colorScheme.onSurfaceVariant
                                },
                            )
                        }
                    }
                } else {
                    items(state.items, key = { "${it.type}:${it.id}" }) { item ->
                        TrashRow(
                            item = item,
                            selectionMode = state.selectionMode,
                            selected = item.id in state.selectedIds,
                            onRestore = { viewModel.restore(item.id) },
                            onDelete = { deleteTarget = item },
                            onEnterSelection = { viewModel.enterSelection(item.id) },
                            onToggleSelection = { viewModel.toggleSelection(item.id) },
                        )
                    }
                }
            }
        }
    }

    deleteTarget?.let { target ->
        AlertDialog(
            onDismissRequest = { deleteTarget = null },
            title = { Text("Delete forever?") },
            text = { Text("“${target.name}” will be permanently deleted. This can't be undone.") },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.deleteForever(target.id)
                    deleteTarget = null
                }) { Text("Delete forever") }
            },
            dismissButton = {
                TextButton(onClick = { deleteTarget = null }) { Text("Cancel") }
            },
        )
    }

    if (confirmEmpty) {
        val count = state.items.size
        AlertDialog(
            onDismissRequest = { confirmEmpty = false },
            title = { Text("Empty trash?") },
            text = {
                Text("$count item(s) will be permanently deleted. This can't be undone.")
            },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.emptyTrash()
                    confirmEmpty = false
                }) { Text("Empty trash") }
            },
            dismissButton = {
                TextButton(onClick = { confirmEmpty = false }) { Text("Cancel") }
            },
        )
    }

    if (confirmDeleteSelected) {
        val count = state.selectedIds.size
        AlertDialog(
            onDismissRequest = { confirmDeleteSelected = false },
            title = { Text("Delete forever?") },
            text = {
                Text("$count item(s) will be permanently deleted. This can't be undone.")
            },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.deleteSelectedForever()
                    confirmDeleteSelected = false
                }) { Text("Delete forever") }
            },
            dismissButton = {
                TextButton(onClick = { confirmDeleteSelected = false }) { Text("Cancel") }
            },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
private fun TrashRow(
    item: TrashItem,
    selectionMode: Boolean,
    selected: Boolean,
    onRestore: () -> Unit,
    onDelete: () -> Unit,
    onEnterSelection: () -> Unit,
    onToggleSelection: () -> Unit,
) {
    val haptics = LocalHapticFeedback.current
    // Highlight selected rows; transparent otherwise so the ListItem keeps its default surface.
    val rowColor = if (selected) {
        MaterialTheme.colorScheme.secondaryContainer
    } else {
        Color.Transparent
    }
    ListItem(
        colors = ListItemDefaults.colors(containerColor = rowColor),
        modifier = Modifier.combinedClickable(
            onClick = { if (selectionMode) onToggleSelection() },
            onLongClick = {
                if (!selectionMode) {
                    haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                    onEnterSelection()
                }
            },
        ),
        headlineContent = { Text(item.name, maxLines = 1) },
        supportingContent = {
            val deleted = relativeDeleted(item.deletedAtMillis)
            val text = if (item.isFolder) deleted else "${ByteFormatter.format(item.sizeBytes)} • $deleted"
            Text(text)
        },
        leadingContent = {
            if (selectionMode) {
                // The whole row toggles selection; describe state on the checkbox and
                // let the row's click handle the action (null onCheckedChange).
                Checkbox(
                    checked = selected,
                    onCheckedChange = null,
                    modifier = Modifier.clearAndSetSemantics {},
                )
            } else {
                Icon(
                    if (item.isFolder) Icons.Filled.Folder else Icons.Filled.InsertDriveFile,
                    contentDescription = if (item.isFolder) "Folder" else "File",
                )
            }
        },
        trailingContent = if (selectionMode) {
            null
        } else {
            {
                Row {
                    IconButton(onClick = onRestore) {
                        Icon(Icons.Filled.Restore, contentDescription = "Restore ${item.name}")
                    }
                    IconButton(onClick = onDelete) {
                        Icon(Icons.Filled.DeleteForever, contentDescription = "Delete ${item.name} forever")
                    }
                }
            }
        },
    )
}

/**
 * The model only carries when an item was deleted (no expiry window is exposed),
 * so show a human-readable "deleted N ago" relative to now.
 */
private fun relativeDeleted(deletedAtMillis: Long): String {
    val elapsed = max(0L, System.currentTimeMillis() - deletedAtMillis)
    val minutes = elapsed / 60_000
    val hours = minutes / 60
    val days = hours / 24
    return when {
        days >= 1 -> "deleted ${days}d ago"
        hours >= 1 -> "deleted ${hours}h ago"
        minutes >= 1 -> "deleted ${minutes}m ago"
        else -> "deleted just now"
    }
}
