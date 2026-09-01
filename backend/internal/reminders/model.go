package reminders

type Reminder struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	PetID     string `json:"petId"`
	Title     string `json:"title"`
	DueDate   string `json:"dueDate"`
	DaysUntil int    `json:"daysUntil"`
}
