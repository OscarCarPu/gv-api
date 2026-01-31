# Habits

### handler_test.go

- `TestUpsertLog_Success` - Verifies successful log upsert returns 200 and passes correct data to service
- `TestUpsertLog_InvalidJSON` - Verifies malformed JSON body returns 400 Bad Request
- `TestUpsertLog_ServiceError` - Verifies service error returns 500 Internal Server Error
- `TestGetDaily_ReturnsJSONArray` - Verifies habits are returned as JSON array with correct content-type
- `TestGetDaily_ServiceError` - Verifies service error returns 500 Internal Server Error
- `TestCreateHabit_Success` - Verifies successful habit creation returns 201 Created
- `TestCreateHabit_InvalidJSON` - Verifies malformed JSON body returns 400 Bad Request
- `TestCreateHabit_MissingName` - Verifies missing name field returns 400 with validation error
- `TestCreateHabit_ServiceError` - Verifies service error returns 500 Internal Server Error

### service_test.go

- `TestGetDailyView_DateParsing` - Verifies date string is parsed correctly and invalid dates return error
- `TestGetDailyView_EmptyResults` - Verifies empty habit list is handled correctly
- `TestGetDailyView_EmptyDateUsesToday` - Verifies empty date string defaults to current date
- `TestLogHabit_DelegatesToRepo` - Verifies request data is correctly passed to repository
- `TestLogHabit_InvalidDate` - Verifies invalid date format returns error
- `TestCreateHabit_DelegatesToRepo` - Verifies name and description are passed to repository
- `TestCreateHabit_NilDescription` - Verifies nil description is handled correctly

# Response

### response_test.go

- `TestJSON_SetsContentTypeAndStatus` - Verifies Content-Type header and status code are set correctly
- `TestJSON_EncodesStruct` - Verifies structs are properly JSON encoded in response body
- `TestError_ReturnsErrorJSON` - Verifies error message is wrapped in JSON object with error key
- `TestError_DifferentStatusCodes` - Verifies various HTTP status codes work correctly
