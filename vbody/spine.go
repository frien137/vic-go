package vbody

// #cgo LDFLAGS: -L${SRCDIR}/.. -lrobot
// #cgo CFLAGS: -I${SRCDIR}/../include
// #include "librobot.h"
// #include "spine.h"
import "C"
import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// bgr? red is finnicky for some reason
	LED_GREEN = 0x00FF00
	LED_RED   = 0x0000FF
	LED_BLUE  = 0xFF0000
	LED_OFF   = 0x0
)

var CurrentDataFrame DataFrame

type MotorStatus [4]struct {
	Pos int32
	DLT int32
	TM  uint32
}

type DataFrame struct {
	mu             sync.Mutex
	Seq            uint32
	Cliffs         [4]uint32
	Encoders       MotorStatus
	BattVoltage    int16
	ChargerVoltage int16
	BodyTemp       int16
	ProxMM         uint16
	Touch          uint16
	ButtonState    bool
	MicData        []int16
}

var Spine_Handle int
var Spine_Initiated bool
var Motor_1 int16
var Motor_2 int16
var Motor_3 int16
var Motor_4 int16
var FrontLEDStatus uint32 = LED_OFF
var MiddleLEDStatus uint32 = LED_OFF
var BackLEDStatus uint32 = LED_OFF

// returns a handle for debugging, though the cpp should now handle it on its own
func Init_Spine() int {
	if Spine_Initiated {
		fmt.Println("Spine already initiated, handle " + fmt.Sprint(Spine_Handle))
		return Spine_Handle
	}
	handle := C.spine_full_init()
	if handle > 0 {
		Spine_Initiated = true
		Start_Comms_Loop()
	} else {
		Spine_Initiated = false
	}
	return int(handle)
}

func Close_Spine() {
	if Spine_Initiated {
		Spine_Initiated = false
		time.Sleep(time.Millisecond * 50)
		C.close_spine()
	}
}

func Set_LEDs(front uint32, middle uint32, back uint32) error {
	if !Spine_Initiated {
		return errors.New("initiate spine first")
	}
	FrontLEDStatus, MiddleLEDStatus, BackLEDStatus = front, middle, back
	return nil
}

// rwheel, lwheel, lift, head
func Start_Comms_Loop() error {
	if !Spine_Initiated {
		return errors.New("initiate spine first")
	}

	// check if body is responding
	// read 10 frames, make sure touch sensor is
	for i := 0; i <= 10; i++ {
		if !Spine_Initiated {
			return errors.New("spine became uninitialized during comms loop start")
		}
		CurrentDataFrame.mu.Lock()
		ReadFrame()
		CurrentDataFrame.mu.Unlock()
		frame := GetFrame()
		if i == 10 && frame.Touch == 0 {
			return errors.New("body hasn't returned a valid frame after " + fmt.Sprint(i) + " tries")
		} else if frame.Touch == 0 {
			time.Sleep(time.Millisecond * 10)
			continue
		} else {
			fmt.Println("Spine data is valid, initiating comms channel...")
			break
		}
	}

	go func() {
		for {
			if !Spine_Initiated {
				return
			}
			var motors []int16 = []int16{Motor_1, Motor_2, Motor_3, Motor_4}
			var leds []uint32 = []uint32{BackLEDStatus, MiddleLEDStatus, FrontLEDStatus, FrontLEDStatus}
			C.spine_full_update(C.uint32_t(8888), (*C.int16_t)(&motors[0]), (*C.uint32_t)(&leds[0]))
			time.Sleep(time.Millisecond * 10)
		}
	}()
	go func() {
		for {
			if !Spine_Initiated {
				return
			}
			CurrentDataFrame.mu.Lock()
			ReadFrame()
			CurrentDataFrame.mu.Unlock()
		}
	}()
	time.Sleep(time.Second)
	fmt.Println("Spine comms initiated")
	return nil
}

// rwheel, lwheel, lift, head
func Set_Motors(m1 int16, m2 int16, m3 int16, m4 int16) error {
	// back: 	vbody.Set_Motors(500, -500, -500, 0)
	m1 = -(m1)
	m3 = -(m3)
	m4 = -(m4)
	if !Spine_Initiated {
		return errors.New("initiate spine first")
	}
	Motor_1, Motor_2, Motor_3, Motor_4 = m1*100, m2*100, m3*100, m4*100
	return nil
}

/*
reading

typedef struct {
    uint32_t seq;
    uint16_t status;
    uint8_t i2c_device_fault;
    uint8_t i2c_fault_item;
    spine_motor_status_t motors[4];
    uint16_t cliff_sensor[4];
    int16_t battery_voltage;
    int16_t charger_voltage;
    int16_t body_temp;
    uint16_t battery_flags;
    uint16_t __reserved1;
    uint8_t prox_sigma_mm;
    uint16_t prox_raw_range_mm;
    uint16_t prox_signal_rate_mcps;
    uint16_t prox_ambient;
    uint16_t prox_SPAD_count;
    uint16_t prox_sample_count;
    uint32_t prox_calibration_result;
    uint16_t touch_sensor;
    uint16_t buttton_state;
    uint32_t mic_indices;
    uint16_t button_inputs;
    uint8_t __reserved2[26];
    uint16_t mic_data[320];
} spine_dataframe_t;

typedef struct {
    int32_t pos;
    int32_t dlt;
    uint32_t tm;
} spine_motor_status_t;
*/

func GetFrame() *DataFrame {
	CurrentDataFrame.mu.Lock()
	defer CurrentDataFrame.mu.Unlock()
	return &CurrentDataFrame
}

func ReadFrame() {
	df := C.iterate()
	CurrentDataFrame.Seq = uint32(df.seq)
	goms := MotorStatus{}
	ms := df.motors
	for i := range ms {
		goms[i].Pos = int32(ms[i].pos)
		goms[i].DLT = int32(ms[i].dlt)
		goms[i].TM = uint32(ms[i].tm)
	}
	CurrentDataFrame.Encoders = goms
	CurrentDataFrame.Cliffs = [4]uint32{uint32(df.cliff_sensor[0]), uint32(df.cliff_sensor[1]), uint32(df.cliff_sensor[2]), uint32(df.cliff_sensor[3])}
	CurrentDataFrame.BattVoltage = int16(df.battery_voltage)
	CurrentDataFrame.ChargerVoltage = int16(df.charger_voltage)
	CurrentDataFrame.BodyTemp = int16(df.body_temp)
	CurrentDataFrame.ProxMM = uint16(df.prox_raw_range_mm)
	CurrentDataFrame.Touch = uint16(df.touch_sensor)
	switch {
	case df.buttton_state > 0:
		CurrentDataFrame.ButtonState = true
	default:
		CurrentDataFrame.ButtonState = false
	}
	for _, data := range df.mic_data {
		CurrentDataFrame.MicData = append(CurrentDataFrame.MicData, int16(data))
	}
}
