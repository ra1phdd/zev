package locale

// Named provider command messages.
const (
	KeyProviderShort         Key = "provider.short"
	KeyProviderListShort     Key = "provider.list_short"
	KeyProviderUseShort      Key = "provider.use_short"
	KeyProviderAddShort      Key = "provider.add_short"
	KeyProviderRemoveShort   Key = "provider.remove_short"
	KeyProviderListEmpty     Key = "provider.list_empty"
	KeyProviderListEntry     Key = "provider.list_entry"
	KeyProviderUseSuccess    Key = "provider.use_success"
	KeyProviderRemoveSuccess Key = "provider.remove_success"
	KeyProviderMissing       Key = "provider.missing"
	KeyProviderActiveRemoval Key = "provider.active_removal"
	KeyProviderAlreadyExists Key = "provider.already_exists"
)

// Update message keys for branch-aware update flows.
const (
	KeyUpdateMainWarning                    Key = "update.main_warning"
	KeyUpdateMainStableCompatibilityWarning Key = "update.main_stable_compatibility_warning"
	KeyUpdateAvailable                      Key = "update.available"
	KeyUpdateAvailableMain                  Key = "update.available_main"
)
