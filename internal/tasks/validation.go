package tasks

// Request-shape validation shared by the create/update handlers.
// Each helper returns "" when valid, or the client-facing error message.

func validateName(name string) string {
	if name == "" {
		return "name is required"
	}
	return validateOptionalName(&name)
}

func validateOptionalName(name *string) string {
	if name != nil && len(*name) > 40 {
		return "name must be at most 40 characters"
	}
	return ""
}

func validateTaskType(taskType *string) string {
	if taskType == nil {
		return ""
	}
	switch *taskType {
	case "standard", "continuous", "recurring":
		return ""
	default:
		return "task_type must be standard, continuous, or recurring"
	}
}

func validatePriority(p *int32) string {
	if p != nil && (*p < 1 || *p > 5) {
		return "priority must be between 1 and 5"
	}
	return ""
}

func validateRecurrenceValue(recurrence *int32) string {
	if recurrence != nil && *recurrence <= 0 {
		return "recurrence must be a positive number of days"
	}
	return ""
}
