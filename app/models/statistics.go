package models

// BookingStatistics содержит агрегированную аналитику.
type BookingStatistics struct {
	TotalCount   int
	Statuses     map[string]int
	TopResources []ResourceStatistic
}

// ResourceStatistic содержит статистику по конкретному ресурсу.
type ResourceStatistic struct {
	ResourceID int64
	Count      int
}
