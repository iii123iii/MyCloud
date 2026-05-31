package com.mycloud.feature.trash

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DeleteForever
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.InsertDriveFile
import androidx.compose.material.icons.filled.Restore
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.common.util.ByteFormatter
import com.mycloud.core.model.TrashItem

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TrashScreen(
    modifier: Modifier = Modifier,
    viewModel: TrashViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    // Auto-refresh when the screen (re)appears (the VM is Activity-scoped).
    LaunchedEffect(Unit) { viewModel.refresh() }

    androidx.compose.material3.Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                title = { Text("Trash") },
                actions = {
                    if (state.items.isNotEmpty()) {
                        TextButton(onClick = viewModel::restoreAll) { Text("Restore all") }
                        TextButton(onClick = viewModel::emptyTrash) { Text("Empty") }
                    }
                },
            )
        },
    ) { padding ->
        PullToRefreshBox(
            isRefreshing = state.isLoading,
            onRefresh = viewModel::refresh,
            modifier = Modifier.fillMaxSize().padding(padding),
        ) {
            // Always a LazyColumn (even when empty) so the surrounding
            // PullToRefreshBox still gets the pull gesture via nested scroll.
            LazyColumn(Modifier.fillMaxSize()) {
                if (state.items.isEmpty()) {
                    item {
                        Box(Modifier.fillParentMaxSize(), contentAlignment = Alignment.Center) {
                            Text(
                                "Trash is empty.",
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                } else {
                    items(state.items, key = { "${it.type}:${it.id}" }) { item ->
                        TrashRow(
                            item = item,
                            onRestore = { viewModel.restore(item.id) },
                            onDelete = { viewModel.deleteForever(item.id) },
                        )
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TrashRow(item: TrashItem, onRestore: () -> Unit, onDelete: () -> Unit) {
    ListItem(
        headlineContent = { Text(item.name, maxLines = 1) },
        supportingContent = {
            if (!item.isFolder) Text(ByteFormatter.format(item.sizeBytes))
        },
        leadingContent = {
            Icon(
                if (item.isFolder) Icons.Filled.Folder else Icons.Filled.InsertDriveFile,
                contentDescription = null,
            )
        },
        trailingContent = {
            Row {
                IconButton(onClick = onRestore) {
                    Icon(Icons.Filled.Restore, contentDescription = "Restore")
                }
                IconButton(onClick = onDelete) {
                    Icon(Icons.Filled.DeleteForever, contentDescription = "Delete forever")
                }
            }
        },
    )
}
