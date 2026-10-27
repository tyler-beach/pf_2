package main

deny contains msg if {
    object.get(input, "retry_limit", null) == null
    msg := "retry_limit is required"
}

deny contains msg if {
    timeout := object.get(input, "timeout_seconds", null)
    not is_number(timeout)
    msg := "timeout_seconds must be numeric"
}

deny contains msg if {
    timeout := object.get(input, "timeout_seconds", null)
    is_number(timeout)
    timeout < 1
    msg := "timeout_seconds must be at least 1"
}

deny contains msg if {
    timeout := object.get(input, "timeout_seconds", null)
    is_number(timeout)
    timeout > 120
    msg := "timeout_seconds must be at most 120"
}
