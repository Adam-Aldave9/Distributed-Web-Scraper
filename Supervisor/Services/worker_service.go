package Services

import (
	"time"

	Models "supervisor/Models"
)

func GetAllWorkers() ([]Models.Worker, error) {
	var workers []Models.Worker
	result := Models.WorkerDB.Find(&workers)
	if result.Error != nil {
		return nil, result.Error
	}
	return workers, nil
}

func GetWorkerById(id int) (Models.Worker, error) {
	var worker Models.Worker
	result := Models.WorkerDB.First(&worker, id)
	if result.Error != nil {
		return Models.Worker{}, result.Error
	}
	return worker, nil
}

func CreateWorker(worker Models.Worker) (Models.Worker, error) {
	worker.Status = "online"
	worker.LastHeartbeat = time.Now()
	result := Models.WorkerDB.Create(&worker)
	if result.Error != nil {
		return Models.Worker{}, result.Error
	}
	return worker, nil
}

func UpdateWorker(worker Models.Worker) (Models.Worker, error) {
	worker.LastHeartbeat = time.Now()
	worker.Status = "online"
	result := Models.WorkerDB.Save(&worker)
	if result.Error != nil {
		return Models.Worker{}, result.Error
	}
	return worker, nil
}

func DeleteWorker(id int) (string, error) {
	var worker Models.Worker
	result := Models.WorkerDB.Delete(&worker, id)
	if result.Error != nil {
		return "", result.Error
	}
	return "Worker deleted successfully", nil
}
