package com.mycloud.feature.trash

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.model.TrashItem
import com.mycloud.data.repository.TrashRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class TrashUiState(
    val items: List<TrashItem> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val selectionMode: Boolean = false,
    val selectedIds: Set<String> = emptySet(),
)

@HiltViewModel
class TrashViewModel @Inject constructor(
    private val trashRepository: TrashRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(TrashUiState())
    val state: StateFlow<TrashUiState> = _state.asStateFlow()

    // No init refresh: the screen's LaunchedEffect drives the (re)load on every
    // entry, so an init block here would just double-fetch on first appearance.

    fun refresh() {
        _state.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            when (val result = trashRepository.list()) {
                is NetworkResult.Success ->
                    _state.update { it.copy(items = result.data, isLoading = false, error = null) }
                // Keep whatever was already shown rather than blanking the list to look "empty".
                else ->
                    _state.update { it.copy(isLoading = false, error = result.userMessageOrNull()) }
            }
        }
    }

    // --- Multi-select (bulk selection) ---

    /** Enter selection mode with [id] pre-selected (typically from a long-press). */
    fun enterSelection(id: String) {
        _state.update { it.copy(selectionMode = true, selectedIds = setOf(id)) }
    }

    /** Add/remove [id] from the selection while in selection mode. */
    fun toggleSelection(id: String) {
        _state.update {
            val next = if (id in it.selectedIds) it.selectedIds - id else it.selectedIds + id
            it.copy(selectedIds = next)
        }
    }

    /** Select every currently shown item. */
    fun selectAll() {
        _state.update { it.copy(selectionMode = true, selectedIds = it.items.map { item -> item.id }.toSet()) }
    }

    /** Clear the selection but stay in selection mode. */
    fun clearSelection() {
        _state.update { it.copy(selectedIds = emptySet()) }
    }

    /** Leave selection mode and drop any selection. */
    fun exitSelection() {
        _state.update { it.copy(selectionMode = false, selectedIds = emptySet()) }
    }

    /** Restore every selected item concurrently, then exit selection mode. */
    fun restoreSelected() = mutateSelected("Couldn't restore") { trashRepository.restore(it) }

    /** Permanently delete every selected item concurrently, then exit selection mode. */
    fun deleteSelectedForever() = mutateSelected("Couldn't delete") { trashRepository.deleteForever(it) }

    fun restore(id: String) = mutate(id) { trashRepository.restore(id) }

    fun restoreAll() {
        val ids = _state.value.items.map { it.id }
        if (ids.isEmpty()) return
        _state.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            try {
                // Fire all restores concurrently rather than serially, and surface the
                // first failure instead of silently ignoring per-item errors.
                val results = ids.map { id -> async { trashRepository.restore(id) } }.awaitAll()
                val failures = results.count { it !is NetworkResult.Success }
                val opError = if (failures > 0) {
                    results.firstNotNullOfOrNull { it.userMessageOrNull() }
                        ?: "Couldn't restore $failures item(s)."
                } else {
                    null
                }
                // Reconcile with the server, then keep the operation error if any.
                val listResult = trashRepository.list()
                _state.update {
                    when (listResult) {
                        is NetworkResult.Success ->
                            it.copy(items = listResult.data, error = opError)
                        else ->
                            it.copy(error = opError ?: listResult.userMessageOrNull())
                    }
                }
            } finally {
                // Always clear the spinner, even if a call threw or was cancelled.
                _state.update { it.copy(isLoading = false) }
            }
        }
    }

    fun deleteForever(id: String) = mutate(id) { trashRepository.deleteForever(id) }

    fun emptyTrash() {
        _state.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            val result = trashRepository.empty()
            if (result is NetworkResult.Success) {
                refresh()
            } else {
                _state.update { it.copy(isLoading = false, error = result.userMessageOrNull()) }
            }
        }
    }

    /**
     * Optimistically remove the affected item so it doesn't linger for the full
     * server round-trip; restore it (with an error) if the call fails.
     */
    /**
     * Run [action] for every selected id concurrently (same async{}+awaitAll() pattern
     * as [restoreAll]), surface the first failure via [error], reconcile with the
     * server, then always clear the selection and exit selection mode.
     */
    private fun mutateSelected(failurePrefix: String, action: suspend (String) -> NetworkResult<Unit>) {
        val ids = _state.value.selectedIds.toList()
        if (ids.isEmpty()) {
            exitSelection()
            return
        }
        _state.update { it.copy(isLoading = true, error = null) }
        viewModelScope.launch {
            try {
                val results = ids.map { id -> async { action(id) } }.awaitAll()
                val failures = results.count { it !is NetworkResult.Success }
                val opError = if (failures > 0) {
                    results.firstNotNullOfOrNull { it.userMessageOrNull() }
                        ?: "$failurePrefix $failures item(s)."
                } else {
                    null
                }
                val listResult = trashRepository.list()
                _state.update {
                    when (listResult) {
                        is NetworkResult.Success ->
                            it.copy(items = listResult.data, error = opError)
                        else ->
                            it.copy(error = opError ?: listResult.userMessageOrNull())
                    }
                }
            } finally {
                // Always drop the spinner and leave selection mode, even on failure/cancel.
                _state.update { it.copy(isLoading = false, selectionMode = false, selectedIds = emptySet()) }
            }
        }
    }

    private fun mutate(id: String, action: suspend () -> NetworkResult<Unit>) {
        val previous = _state.value.items
        _state.update { it.copy(items = it.items.filterNot { item -> item.id == id }, error = null) }
        viewModelScope.launch {
            val result = action()
            if (result is NetworkResult.Success) {
                refresh()
            } else {
                // Roll the optimistic removal back and surface why.
                _state.update { it.copy(items = previous, error = result.userMessageOrNull()) }
            }
        }
    }
}
