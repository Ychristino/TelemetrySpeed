package database

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TelemetryRow carries all per-car per-frame telemetry for one DB insert.
// Field order mirrors telemetryCols below — keep them in sync.
type TelemetryRow struct {
	SessionID uuid.UUID
	CarIndex  int16
	Timestamp time.Time

	// Driver inputs
	Speed    float32
	Throttle float32
	Brake    float32
	Steering float32
	Gear     int16
	Clutch   float32
	DRS      int16

	// Position / orientation
	PosX, PosY, PosZ float32
	VelX, VelY, VelZ float32
	Yaw, Pitch, Roll  float32

	// G-forces
	GLat, GLong, GVert float32

	// Engine / fuel
	RPM             int32
	EngTemp         float32
	Fuel            float32
	FuelRemLaps     float32
	FuelMix         int16
	PitLimiter      int16
	EnginePowerICE  float32
	EnginePowerMGUK float32

	// ERS
	ERSStore      float32
	ERSDeployMode int16

	// Aids
	TractionControl int16
	ABS             int16

	// Tyres (scalars)
	TyreCompound int16
	TyreAgeLaps  int16

	// Tyre temps (RL, RR, FL, FR)
	TyreSurfTempRL, TyreSurfTempRR, TyreSurfTempFL, TyreSurfTempFR    float32
	TyreInnerTempRL, TyreInnerTempRR, TyreInnerTempFL, TyreInnerTempFR float32
	TyrePressureRL, TyrePressureRR, TyrePressureFL, TyrePressureFR     float32
	TyreWearRL, TyreWearRR, TyreWearFL, TyreWearFR                    float32
	TyreBlisterRL, TyreBlisterRR, TyreBlisterFL, TyreBlisterFR        int16

	// Brakes
	BrakeTempRL, BrakeTempRR, BrakeTempFL, BrakeTempFR      float32
	BrakeDamageRL, BrakeDamageRR, BrakeDamageFL, BrakeDamageFR int16

	// Lap state
	LapTimeMS    int32
	LapDistance  float32
	CarPosition  int16
	LapNum       int16
	Sector       int16
	PitStatus    int16
	DriverStatus int16
	LapInvalid   int16
	Penalties    int16

	// MotionEx — player car only; NULL for other cars when HasMotionEx==false
	HasMotionEx bool

	SuspPosRL, SuspPosRR, SuspPosFL, SuspPosFR     float32
	SuspVelRL, SuspVelRR, SuspVelFL, SuspVelFR     float32
	SuspAccelRL, SuspAccelRR, SuspAccelFL, SuspAccelFR float32

	WheelSpeedRL, WheelSpeedRR, WheelSpeedFL, WheelSpeedFR         float32
	WheelSlipRatioRL, WheelSlipRatioRR, WheelSlipRatioFL, WheelSlipRatioFR float32
	WheelSlipAngleRL, WheelSlipAngleRR, WheelSlipAngleFL, WheelSlipAngleFR float32
	WheelLatForceRL, WheelLatForceRR, WheelLatForceFL, WheelLatForceFR  float32
	WheelLongForceRL, WheelLongForceRR, WheelLongForceFL, WheelLongForceFR float32
	WheelVertForceRL, WheelVertForceRR, WheelVertForceFL, WheelVertForceFR float32

	LocalVelX, LocalVelY, LocalVelZ    float32
	AngVelX, AngVelY, AngVelZ         float32
	FrontWheelsAngle                    float32
	FrontAeroHeight, RearAeroHeight    float32

	WheelCamberRL, WheelCamberRR, WheelCamberFL, WheelCamberFR float32
}

// telemetryCols is the ordered column list for CopyFrom — must match the []any
// slice built in bulkInsert row-by-row.
var telemetryCols = []string{
	"ts", "session_id", "car_index",
	"speed", "throttle", "brake", "steering", "gear", "clutch", "drs",
	"pos_x", "pos_y", "pos_z", "vel_x", "vel_y", "vel_z", "yaw", "pitch", "roll",
	"g_lat", "g_long", "g_vert",
	"rpm", "eng_temp", "fuel", "fuel_rem_laps", "fuel_mix", "pit_limiter",
	"engine_power_ice", "engine_power_mguk",
	"ers_store", "ers_deploy_mode",
	"traction_control", "abs",
	"tyre_compound", "tyre_age_laps",
	"tyre_surf_temp_rl", "tyre_surf_temp_rr", "tyre_surf_temp_fl", "tyre_surf_temp_fr",
	"tyre_inner_temp_rl", "tyre_inner_temp_rr", "tyre_inner_temp_fl", "tyre_inner_temp_fr",
	"tyre_pressure_rl", "tyre_pressure_rr", "tyre_pressure_fl", "tyre_pressure_fr",
	"tyre_wear_rl", "tyre_wear_rr", "tyre_wear_fl", "tyre_wear_fr",
	"tyre_blister_rl", "tyre_blister_rr", "tyre_blister_fl", "tyre_blister_fr",
	"brake_temp_rl", "brake_temp_rr", "brake_temp_fl", "brake_temp_fr",
	"brake_damage_rl", "brake_damage_rr", "brake_damage_fl", "brake_damage_fr",
	"lap_time_ms", "lap_distance", "car_position", "lap_num", "sector",
	"pit_status", "driver_status", "lap_invalid", "penalties",
	"susp_pos_rl", "susp_pos_rr", "susp_pos_fl", "susp_pos_fr",
	"susp_vel_rl", "susp_vel_rr", "susp_vel_fl", "susp_vel_fr",
	"susp_accel_rl", "susp_accel_rr", "susp_accel_fl", "susp_accel_fr",
	"wheel_speed_rl", "wheel_speed_rr", "wheel_speed_fl", "wheel_speed_fr",
	"wheel_slip_ratio_rl", "wheel_slip_ratio_rr", "wheel_slip_ratio_fl", "wheel_slip_ratio_fr",
	"wheel_slip_angle_rl", "wheel_slip_angle_rr", "wheel_slip_angle_fl", "wheel_slip_angle_fr",
	"wheel_lat_force_rl", "wheel_lat_force_rr", "wheel_lat_force_fl", "wheel_lat_force_fr",
	"wheel_long_force_rl", "wheel_long_force_rr", "wheel_long_force_fl", "wheel_long_force_fr",
	"wheel_vert_force_rl", "wheel_vert_force_rr", "wheel_vert_force_fl", "wheel_vert_force_fr",
	"local_vel_x", "local_vel_y", "local_vel_z",
	"ang_vel_x", "ang_vel_y", "ang_vel_z",
	"front_wheels_angle", "front_aero_height", "rear_aero_height",
	"wheel_camber_rl", "wheel_camber_rr", "wheel_camber_fl", "wheel_camber_fr",
}

type TelemetryWriter struct {
	pool          *pgxpool.Pool
	writeCh       chan []TelemetryRow
	flushInterval time.Duration
	batchSize     int
}

func newTelemetryWriter(pool *pgxpool.Pool, flushInterval time.Duration, batchSize int) *TelemetryWriter {
	return &TelemetryWriter{
		pool:          pool,
		writeCh:       make(chan []TelemetryRow, 1024),
		flushInterval: flushInterval,
		batchSize:     batchSize,
	}
}

func (tw *TelemetryWriter) Start(ctx context.Context) {
	go tw.run(ctx)
}

func (tw *TelemetryWriter) WriteCh() chan<- []TelemetryRow {
	return tw.writeCh
}

func (tw *TelemetryWriter) run(ctx context.Context) {
	ticker := time.NewTicker(tw.flushInterval)
	defer ticker.Stop()

	var buf []TelemetryRow

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := tw.bulkInsert(buf); err != nil {
			log.Printf("[DBWriter] insert error: %v", err)
		}
		buf = buf[:0]
	}

	for {
		select {
		case rows := <-tw.writeCh:
			buf = append(buf, rows...)
			if len(buf) >= tw.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			drain := time.After(500 * time.Millisecond)
		drainLoop:
			for {
				select {
				case rows := <-tw.writeCh:
					buf = append(buf, rows...)
				case <-drain:
					break drainLoop
				}
			}
			flush()
			return
		}
	}
}

func (tw *TelemetryWriter) bulkInsert(rows []TelemetryRow) error {
	// optF returns the float32 value as any when the car has MotionEx data,
	// or nil (SQL NULL) otherwise.
	optF := func(has bool, v float32) any {
		if !has {
			return nil
		}
		return v
	}

	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		mx := r.HasMotionEx
		copyRows[i] = []any{
			r.Timestamp, r.SessionID, r.CarIndex,
			// driver inputs
			r.Speed, r.Throttle, r.Brake, r.Steering, r.Gear, r.Clutch, r.DRS,
			// position / orientation
			r.PosX, r.PosY, r.PosZ, r.VelX, r.VelY, r.VelZ, r.Yaw, r.Pitch, r.Roll,
			// g-forces
			r.GLat, r.GLong, r.GVert,
			// engine / fuel
			r.RPM, r.EngTemp, r.Fuel, r.FuelRemLaps, r.FuelMix, r.PitLimiter,
			r.EnginePowerICE, r.EnginePowerMGUK,
			// ERS
			r.ERSStore, r.ERSDeployMode,
			// aids
			r.TractionControl, r.ABS,
			// tyres
			r.TyreCompound, r.TyreAgeLaps,
			r.TyreSurfTempRL, r.TyreSurfTempRR, r.TyreSurfTempFL, r.TyreSurfTempFR,
			r.TyreInnerTempRL, r.TyreInnerTempRR, r.TyreInnerTempFL, r.TyreInnerTempFR,
			r.TyrePressureRL, r.TyrePressureRR, r.TyrePressureFL, r.TyrePressureFR,
			r.TyreWearRL, r.TyreWearRR, r.TyreWearFL, r.TyreWearFR,
			r.TyreBlisterRL, r.TyreBlisterRR, r.TyreBlisterFL, r.TyreBlisterFR,
			// brakes
			r.BrakeTempRL, r.BrakeTempRR, r.BrakeTempFL, r.BrakeTempFR,
			r.BrakeDamageRL, r.BrakeDamageRR, r.BrakeDamageFL, r.BrakeDamageFR,
			// lap state
			r.LapTimeMS, r.LapDistance, r.CarPosition, r.LapNum, r.Sector,
			r.PitStatus, r.DriverStatus, r.LapInvalid, r.Penalties,
			// MotionEx — NULL for non-player cars
			optF(mx, r.SuspPosRL), optF(mx, r.SuspPosRR), optF(mx, r.SuspPosFL), optF(mx, r.SuspPosFR),
			optF(mx, r.SuspVelRL), optF(mx, r.SuspVelRR), optF(mx, r.SuspVelFL), optF(mx, r.SuspVelFR),
			optF(mx, r.SuspAccelRL), optF(mx, r.SuspAccelRR), optF(mx, r.SuspAccelFL), optF(mx, r.SuspAccelFR),
			optF(mx, r.WheelSpeedRL), optF(mx, r.WheelSpeedRR), optF(mx, r.WheelSpeedFL), optF(mx, r.WheelSpeedFR),
			optF(mx, r.WheelSlipRatioRL), optF(mx, r.WheelSlipRatioRR), optF(mx, r.WheelSlipRatioFL), optF(mx, r.WheelSlipRatioFR),
			optF(mx, r.WheelSlipAngleRL), optF(mx, r.WheelSlipAngleRR), optF(mx, r.WheelSlipAngleFL), optF(mx, r.WheelSlipAngleFR),
			optF(mx, r.WheelLatForceRL), optF(mx, r.WheelLatForceRR), optF(mx, r.WheelLatForceFL), optF(mx, r.WheelLatForceFR),
			optF(mx, r.WheelLongForceRL), optF(mx, r.WheelLongForceRR), optF(mx, r.WheelLongForceFL), optF(mx, r.WheelLongForceFR),
			optF(mx, r.WheelVertForceRL), optF(mx, r.WheelVertForceRR), optF(mx, r.WheelVertForceFL), optF(mx, r.WheelVertForceFR),
			optF(mx, r.LocalVelX), optF(mx, r.LocalVelY), optF(mx, r.LocalVelZ),
			optF(mx, r.AngVelX), optF(mx, r.AngVelY), optF(mx, r.AngVelZ),
			optF(mx, r.FrontWheelsAngle), optF(mx, r.FrontAeroHeight), optF(mx, r.RearAeroHeight),
			optF(mx, r.WheelCamberRL), optF(mx, r.WheelCamberRR), optF(mx, r.WheelCamberFL), optF(mx, r.WheelCamberFR),
		}
	}
	n, err := tw.pool.CopyFrom(
		context.Background(),
		pgx.Identifier{"telemetry"},
		telemetryCols,
		pgx.CopyFromRows(copyRows),
	)
	if err == nil {
		log.Printf("[DBWriter] inserted %d telemetry rows", n)
	}
	return err
}
