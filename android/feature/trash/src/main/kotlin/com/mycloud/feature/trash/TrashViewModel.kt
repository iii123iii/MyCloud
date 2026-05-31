package com.mycloud.feature.trash

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.model.TrashItem
import com.mycloud.data.repository.TrashRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class TrashUiState(
    val items: List<TrashItem> = emptyList(),
    val isLoading: Boolean = false,
)

@HiltViewModel
class TrashViewModel @Inject constructor(
    private val trashRepository: TrashRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(TrashUiState())
    val state: StateFlow<TrashUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        _state.update { it.copy(isLoading = true) }
        viewModelScope.launch {
            val items = (trashRepository.list() as? NetworkResult.Success)?.data.orEmpty()
            _state.update { it.copy(items = items, isLoading = false) }
        }
    }

    fun restore(id: String) = mutate { trashRepository.restore(id) }

    fun restoreAll() {
        val ids = _state.value.items.map { it.id }
        if (ids.isEmpty()) return
        _state.update { it.copy(isLoading = true) }
        viewModelScope.launch {
            ids.forEach { trashRepository.restore(it) }
            refresh()
        }
    }

    fun deleteForever(id: String) = mutate { trashRepository.deleteForever(id) }

    fun emptyTrash() = mutate { trashRepository.empty() }

    private fun mutate(action: suspend () -> NetworkResult<Unit>) {
        viewModelScope.launch {
            action()
            refresh()
        }
    }
}
