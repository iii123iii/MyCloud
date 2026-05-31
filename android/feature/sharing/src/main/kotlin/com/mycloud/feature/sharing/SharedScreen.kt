package com.mycloud.feature.sharing

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
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
    viewModel: SharedViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val clipboard = LocalClipboardManager.current
    var tab by rememberSaveable { mutableIntStateOf(0) }

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
        }
        PullToRefreshBox(
            isRefreshing = state.isLoading,
            onRefresh = { viewModel.refresh() },
            modifier = Modifier.fillMaxSize(),
        ) {
            val isEmpty = if (tab == 0) state.shares.isEmpty() else state.grants.isEmpty()
            when {
                // A genuine failure is distinct from an empty list.
                state.error != null && isEmpty ->
                    CenteredMessage(state.error!!, isError = true)
                tab == 0 && isEmpty && !state.isLoading ->
                    CenteredMessage("No share links yet.")
                tab == 1 && isEmpty && !state.isLoading ->
                    CenteredMessage("You haven't granted access to anyone.")
                tab == 0 ->
                    LazyColumn(Modifier.fillMaxSize()) {
                        items(state.shares, key = { it.id }) { share ->
                            ShareRow(
                                share = share,
                                onCopy = { clipboard.setText(AnnotatedString(viewModel.publicLink(share.token))) },
                                onRevoke = { viewModel.revokeShare(share.id) },
                            )
                        }
                    }
                else ->
                    LazyColumn(Modifier.fillMaxSize()) {
                        items(state.grants, key = { it.id }) { grant ->
                            GrantRow(grant = grant, onRevoke = { viewModel.revokeGrant(grant.id) })
                        }
                    }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ShareRow(share: Share, onCopy: () -> Unit, onRevoke: () -> Unit) {
    ListItem(
        headlineContent = { Text(share.fileName ?: "Shared item", maxLines = 1) },
        supportingContent = {
            Text("${share.permission.name.lowercase()} • ${share.downloadCount} downloads")
        },
        trailingContent = {
            androidx.compose.foundation.layout.Row {
                IconButton(onClick = onCopy) {
                    Icon(Icons.Filled.ContentCopy, contentDescription = "Copy link")
                }
                IconButton(onClick = onRevoke) {
                    Icon(Icons.Filled.Delete, contentDescription = "Revoke")
                }
            }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GrantRow(grant: Grant, onRevoke: () -> Unit) {
    ListItem(
        headlineContent = { Text(grant.granteeName ?: grant.granteeUserId, maxLines = 1) },
        supportingContent = { Text(grant.permission.name.lowercase()) },
        trailingContent = {
            IconButton(onClick = onRevoke) {
                Icon(Icons.Filled.Delete, contentDescription = "Revoke")
            }
        },
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
