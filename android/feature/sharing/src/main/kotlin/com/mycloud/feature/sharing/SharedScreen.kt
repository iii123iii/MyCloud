package com.mycloud.feature.sharing

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.model.Grant
import com.mycloud.core.model.Share

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SharedScreen(
    modifier: Modifier = Modifier,
    scrollToTopSignal: Int = 0,
    viewModel: SharedViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val clipboard = LocalClipboardManager.current
    var tab by rememberSaveable { mutableIntStateOf(0) }
    // Shared by the active tab's list (only one is composed at a time).
    val listState = rememberLazyListState()

    // Re-tapping the Shared tab scrolls the active list back to the top.
    LaunchedEffect(scrollToTopSignal) {
        if (scrollToTopSignal > 0) listState.animateScrollToItem(0)
    }

    // Revoke confirmation targets (null = no dialog).
    var revokeShareTarget by remember { mutableStateOf<Share?>(null) }
    var revokeGrantTarget by remember { mutableStateOf<Grant?>(null) }

    // Refresh whenever the screen is (re)entered so newly-created shares/grants show.
    LaunchedEffect(Unit) { viewModel.refresh() }

    Column(
        modifier
            .fillMaxSize()
            .statusBarsPadding(),
    ) {
        TabRow(selectedTabIndex = tab) {
            Tab(selected = tab == 0, onClick = { tab = 0 }, text = { Text("Links") })
            Tab(selected = tab == 1, onClick = { tab = 1 }, text = { Text("People") })
            Tab(selected = tab == 2, onClick = { tab = 2 }, text = { Text("Shared with me") })
        }
        PullToRefreshBox(
            isRefreshing = state.isLoading,
            onRefresh = { viewModel.refresh() },
            modifier = Modifier.fillMaxSize(),
        ) {
            val isEmpty = when (tab) {
                0 -> state.shares.isEmpty()
                1 -> state.grants.isEmpty()
                else -> state.incomingGrants.isEmpty()
            }
            val tabError = when (tab) {
                0 -> state.sharesError
                1 -> state.grantsError
                else -> state.incomingGrantsError
            }
            when {
                // A genuine failure is distinct from an empty list.
                tabError != null && isEmpty ->
                    CenteredMessage(tabError, isError = true)
                tab == 0 && isEmpty && !state.isLoading ->
                    CenteredMessage("No share links yet.")
                tab == 1 && isEmpty && !state.isLoading ->
                    CenteredMessage("You haven't granted access to anyone.")
                tab == 2 && isEmpty && !state.isLoading ->
                    CenteredMessage("Nothing has been shared with you.")
                tab == 0 ->
                    LazyColumn(modifier = Modifier.fillMaxSize(), state = listState) {
                        items(state.shares, key = { it.id }) { share ->
                            ShareRow(
                                share = share,
                                onCopy = { clipboard.setText(AnnotatedString(viewModel.publicLink(share.token))) },
                                onRevoke = { revokeShareTarget = share },
                            )
                        }
                    }
                tab == 1 ->
                    LazyColumn(modifier = Modifier.fillMaxSize(), state = listState) {
                        items(state.grants, key = { it.id }) { grant ->
                            GrantRow(grant = grant, onRevoke = { revokeGrantTarget = grant })
                        }
                    }
                else ->
                    LazyColumn(modifier = Modifier.fillMaxSize(), state = listState) {
                        items(state.incomingGrants, key = { it.id }) { grant ->
                            IncomingGrantRow(grant = grant)
                        }
                    }
            }
        }
    }

    revokeShareTarget?.let { share ->
        val label = share.fileName ?: "this share link"
        AlertDialog(
            onDismissRequest = { revokeShareTarget = null },
            title = { Text("Revoke link?") },
            text = { Text("Anyone with the link to “$label” will lose access.") },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.revokeShare(share.id)
                    revokeShareTarget = null
                }) { Text("Revoke") }
            },
            dismissButton = {
                TextButton(onClick = { revokeShareTarget = null }) { Text("Cancel") }
            },
        )
    }

    revokeGrantTarget?.let { grant ->
        val label = grant.granteeName ?: grant.granteeUserId
        AlertDialog(
            onDismissRequest = { revokeGrantTarget = null },
            title = { Text("Revoke access?") },
            text = { Text("$label will lose access.") },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.revokeGrant(grant.id)
                    revokeGrantTarget = null
                }) { Text("Revoke") }
            },
            dismissButton = {
                TextButton(onClick = { revokeGrantTarget = null }) { Text("Cancel") }
            },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ShareRow(share: Share, onCopy: () -> Unit, onRevoke: () -> Unit) {
    val label = share.fileName ?: "Shared item"
    ListItem(
        headlineContent = { Text(label, maxLines = 1) },
        supportingContent = {
            // Surface limit/expiry when present so a constrained link is recognizable.
            val parts = buildList {
                add("${share.permission.name.lowercase()} • ${share.downloadCount} downloads")
                share.downloadLimit?.let { add("limit $it") }
                share.expiresAtMillis?.let {
                    add(
                        java.text.SimpleDateFormat("expires MMM d", java.util.Locale.getDefault())
                            .format(java.util.Date(it)),
                    )
                }
            }
            Text(parts.joinToString(" • "))
        },
        trailingContent = {
            androidx.compose.foundation.layout.Row {
                IconButton(onClick = onCopy) {
                    Icon(Icons.Filled.ContentCopy, contentDescription = "Copy link to $label")
                }
                IconButton(onClick = onRevoke) {
                    Icon(Icons.Filled.Delete, contentDescription = "Revoke link to $label")
                }
            }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GrantRow(grant: Grant, onRevoke: () -> Unit) {
    val label = grant.granteeName ?: grant.granteeUserId
    ListItem(
        headlineContent = { Text(label, maxLines = 1) },
        supportingContent = { Text(grant.permission.name.lowercase()) },
        trailingContent = {
            IconButton(onClick = onRevoke) {
                Icon(Icons.Filled.Delete, contentDescription = "Revoke access for $label")
            }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun IncomingGrantRow(grant: Grant) {
    // Read-only: a recipient can't revoke the owner's grant, so there is no
    // trailing action. Identify the shared item by name, falling back to its id.
    val label = grant.fileName ?: grant.folderName ?: grant.fileId ?: grant.folderId ?: "Shared item"
    val sharedBy = grant.granterName?.let { "shared by $it" } ?: "shared with you"
    ListItem(
        headlineContent = { Text(label, maxLines = 1) },
        supportingContent = { Text("$sharedBy • ${grant.permission.name.lowercase()}") },
    )
}

@Composable
private fun CenteredMessage(message: String, isError: Boolean = false) {
    // A LazyColumn (even with one item) is a nested-scroll container, so the
    // surrounding PullToRefreshBox still responds to a pull when the list is empty.
    LazyColumn(Modifier.fillMaxSize()) {
        item {
            Box(Modifier.fillParentMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                Text(
                    message,
                    color = if (isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
