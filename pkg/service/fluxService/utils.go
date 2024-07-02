package fluxService

// extractValuesFromRowCells extracting fields from the list of fluxAppList rowCells based on the columnDefinitions fields,
func extractValuesFromRowCells(rowCells []interface{}, columnDefinitions map[string]int) (string, string, string) {
	getValue := func(index int) string {
		valueAtIndex := "Unknown"
		if index < len(rowCells) {
			if value, ok := rowCells[index].(string); ok && value != "" {
				valueAtIndex = value
				return valueAtIndex
			}
		}
		return valueAtIndex
	}
	name := getValue(columnDefinitions[AppNameKey])
	syncStatus := getValue(columnDefinitions[StatusKey])
	healthStatus := getValue(columnDefinitions[ReadyKey])
	return name, syncStatus, healthStatus
}
