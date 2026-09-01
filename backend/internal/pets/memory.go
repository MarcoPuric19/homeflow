package pets

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu           sync.Mutex
	items        map[string]Pet
	records      map[string][]MedicalRecord
	vaccinations map[string][]Vaccination
	medications  map[string][]PetMedication
	weights      map[string][]WeightEntry
	labResults   map[string][]LabResult
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: map[string]Pet{}, records: map[string][]MedicalRecord{}, vaccinations: map[string][]Vaccination{}, medications: map[string][]PetMedication{}, weights: map[string][]WeightEntry{}, labResults: map[string][]LabResult{}}
}

func (r *MemoryRepository) CreateWeight(_ context.Context, _, _ string, e WeightEntry) (WeightEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[e.PetID]; !ok {
		return WeightEntry{}, ErrNotFound
	}
	r.weights[e.PetID] = append(r.weights[e.PetID], e)
	return e, nil
}
func (r *MemoryRepository) ListWeights(_ context.Context, _ string, petID string, limit int) ([]WeightEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[petID]; !ok {
		return nil, ErrNotFound
	}
	items := append([]WeightEntry(nil), r.weights[petID]...)
	sort.Slice(items, func(i, j int) bool { return items[i].MeasuredOn > items[j].MeasuredOn })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
func (r *MemoryRepository) CreateLabResult(_ context.Context, _, _ string, e LabResult) (LabResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[e.PetID]; !ok {
		return LabResult{}, ErrNotFound
	}
	r.labResults[e.PetID] = append(r.labResults[e.PetID], e)
	return e, nil
}
func (r *MemoryRepository) ListLabResults(_ context.Context, _ string, petID string, limit int) ([]LabResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[petID]; !ok {
		return nil, ErrNotFound
	}
	items := append([]LabResult(nil), r.labResults[petID]...)
	sort.Slice(items, func(i, j int) bool { return items[i].MeasuredOn > items[j].MeasuredOn })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *MemoryRepository) Dashboard(_ context.Context, _ string, petID, month string, _ time.Time) (Dashboard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pet, ok := r.items[petID]
	if !ok {
		return Dashboard{}, ErrNotFound
	}
	d := Dashboard{PetID: petID, PetName: pet.Name, Month: month, MedicalRecordCount: len(r.records[petID]), ActiveMedicationCount: len(r.medications[petID])}
	if items := r.weights[petID]; len(items) > 0 {
		value, date := items[len(items)-1].WeightKg, items[len(items)-1].MeasuredOn
		d.LatestWeightKg = &value
		d.LatestWeightDate = &date
	}
	return d, nil
}

func (r *MemoryRepository) CreatePetMedication(_ context.Context, _, _, _ string, medication PetMedication) (PetMedication, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[medication.PetID]; !ok {
		return PetMedication{}, ErrNotFound
	}
	r.medications[medication.PetID] = append(r.medications[medication.PetID], medication)
	return medication, nil
}
func (r *MemoryRepository) ListPetMedications(_ context.Context, _ string, petID string, includeInactive bool, limit int) (PetMedicationPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[petID]; !ok {
		return PetMedicationPage{}, ErrNotFound
	}
	page := PetMedicationPage{Medications: make([]PetMedication, 0)}
	for _, item := range r.medications[petID] {
		if !includeInactive && !item.Active {
			continue
		}
		page.Medications = append(page.Medications, item)
		if len(page.Medications) == limit {
			break
		}
	}
	return page, nil
}
func (r *MemoryRepository) Create(_ context.Context, _, _ string, pet Pet) (Pet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[pet.ID] = pet
	return pet, nil
}
func (r *MemoryRepository) List(_ context.Context, _ string, includeInactive bool, limit int, after string) (Page, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.items))
	for key := range r.items {
		ids = append(ids, key)
	}
	sort.Strings(ids)
	page := Page{Pets: make([]Pet, 0, limit)}
	for _, key := range ids {
		pet := r.items[key]
		if key <= after || (!includeInactive && !pet.Active) {
			continue
		}
		page.Pets = append(page.Pets, pet)
		if len(page.Pets) == limit {
			page.NextCursor = key
			break
		}
	}
	return page, nil
}
func (r *MemoryRepository) Get(_ context.Context, _ string, petID string) (Pet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pet, ok := r.items[petID]
	if !ok {
		return Pet{}, ErrNotFound
	}
	return pet, nil
}
func (r *MemoryRepository) Update(_ context.Context, _ string, pet Pet) (Pet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.items[pet.ID]
	if !ok {
		return Pet{}, ErrNotFound
	}
	pet.CreatedAt = old.CreatedAt
	r.items[pet.ID] = pet
	return pet, nil
}

func (r *MemoryRepository) CreateMedicalRecord(_ context.Context, _, _ string, record MedicalRecord) (MedicalRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[record.PetID]; !ok {
		return MedicalRecord{}, ErrNotFound
	}
	r.records[record.PetID] = append(r.records[record.PetID], record)
	return record, nil
}

func (r *MemoryRepository) ListMedicalRecords(_ context.Context, _ string, petID string, limit int, cursor *medicalCursor) (MedicalRecordPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[petID]; !ok {
		return MedicalRecordPage{}, ErrNotFound
	}
	records := append([]MedicalRecord(nil), r.records[petID]...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].OccurredOn == records[j].OccurredOn {
			return records[i].ID > records[j].ID
		}
		return records[i].OccurredOn > records[j].OccurredOn
	})
	page := MedicalRecordPage{Records: make([]MedicalRecord, 0, limit)}
	for _, record := range records {
		if cursor != nil && (record.OccurredOn > cursor.OccurredOn || (record.OccurredOn == cursor.OccurredOn && record.ID >= cursor.ID)) {
			continue
		}
		page.Records = append(page.Records, record)
		if len(page.Records) > limit {
			last := page.Records[limit-1]
			page.NextCursor = encodeMedicalCursor(medicalCursor{OccurredOn: last.OccurredOn, ID: last.ID})
			page.Records = page.Records[:limit]
			break
		}
	}
	return page, nil
}

func (r *MemoryRepository) CreateVaccination(_ context.Context, _, _ string, vaccination Vaccination) (Vaccination, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[vaccination.PetID]; !ok {
		return Vaccination{}, ErrNotFound
	}
	r.vaccinations[vaccination.PetID] = append(r.vaccinations[vaccination.PetID], vaccination)
	return vaccination, nil
}

func (r *MemoryRepository) ListVaccinations(_ context.Context, _ string, petID string, limit int, cursor *vaccinationCursor) (VaccinationPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[petID]; !ok {
		return VaccinationPage{}, ErrNotFound
	}
	items := append([]Vaccination(nil), r.vaccinations[petID]...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].AdministeredOn == items[j].AdministeredOn {
			return items[i].ID > items[j].ID
		}
		return items[i].AdministeredOn > items[j].AdministeredOn
	})
	page := VaccinationPage{Vaccinations: make([]Vaccination, 0, limit)}
	for _, item := range items {
		if cursor != nil && (item.AdministeredOn > cursor.AdministeredOn || (item.AdministeredOn == cursor.AdministeredOn && item.ID >= cursor.ID)) {
			continue
		}
		page.Vaccinations = append(page.Vaccinations, item)
		if len(page.Vaccinations) > limit {
			last := page.Vaccinations[limit-1]
			page.NextCursor = encodeVaccinationCursor(vaccinationCursor{AdministeredOn: last.AdministeredOn, ID: last.ID})
			page.Vaccinations = page.Vaccinations[:limit]
			break
		}
	}
	return page, nil
}
