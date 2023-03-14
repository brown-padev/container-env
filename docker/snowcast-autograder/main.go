package main

import (
	"fmt"
	"log"
	"snowcast-autograder/mock"
	"snowcast-autograder/student"
	"time"
)

func main() {
	// TestStudentServer()
	// TestStudentControl()
	TestMockServer()
	// TestMockControl()
}

func TestStudentServer() {
	ss, err := student.NewServer(
		10000,
		[]string{"/tmp/Beethoven-SymphonyNo5.mp3", "/tmp/ManchurianCandidates-Breakin.mp3"},
	)
	if err != nil {
		log.Fatal("1\t", err)
	}
	go ss.Start()

	time.Sleep(time.Second)

	for _, line := range ss.FlushStdout() {
		fmt.Print(line)
	}

	stations, err := ss.GetStations("/tmp/stations")
	if err != nil {
		log.Fatal("2\t", err)
	}
	fmt.Printf("got %v stations\n", len(stations))
	for _, s := range stations {
		fmt.Printf("%+v\n", s)
	}

	fmt.Println("sleeping")
	time.Sleep(5 * time.Second)
	fmt.Println("woke up")

	fmt.Printf("got %v stations now\n", len(stations))
	stations, err = ss.GetStations("/tmp/stations1")
	if err != nil {
		log.Fatal("3\t", err)
	}
	for _, s := range stations {
		fmt.Printf("%+v\n", s)
	}

	if err := ss.Quit(); err != nil {
		log.Fatal("4\t", err)
	}
}

func TestStudentControl() {
	sc, err := student.NewControl("localhost", 10000, 9000)
	if err != nil {
		log.Fatal(err)
	}
	go sc.Start()

	time.Sleep(time.Second)

	for _, line := range sc.FlushStdout() {
		fmt.Print(line)
	}
	if err := sc.SetStation(0); err != nil {
		log.Fatal(err)
	}
	for _, line := range sc.FlushStdout() {
		fmt.Print(line)
	}

	time.Sleep(time.Second)

	if err := sc.Quit(); err != nil {
		log.Fatal(err)
	}

	for _, line := range sc.FlushStdout() {
		fmt.Print(line)
	}

	fmt.Println(sc.IsDone())
}

func TestMockServer() {
	ms, err := mock.NewServer(10000, []string{"go.mod"})
	if err != nil {
		log.Fatal(err)
	}
	addr, err := ms.Accept(0)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(addr)

	if h, err := ms.GetHello(addr); err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf("%+v\n", h)
	}
	if err := ms.Welcome(addr); err != nil {
		log.Fatal(err)
	}
	// time.Sleep(time.Millisecond)
	// if err := ms.Welcome(addr); err != nil {
	// log.Fatal(err)
	// }

	// time.Sleep(5 * time.Second)
	// if ss, err := ms.GetSetStation(addr); err != nil {
	// log.Fatal(err)
	// } else {
	// fmt.Printf("%+v\n", ss)
	// }
	// if err := ms.Announce(addr, "go.mod"); err != nil {
	// log.Fatal(err)
	// }

	if err := ms.InvalidCommand(addr, "invalid command test"); err != nil {
		log.Fatal(err)
	}

	time.Sleep(5 * time.Second)

	// if ss, err := ms.GetSetStation(addr); err != nil {
	// log.Fatal(err)
	// } else {
	// fmt.Printf("%+v\n", ss)
	// }
	// if err := ms.Announce(addr, "go.mod"); err != nil {
	// log.Fatal(err)
	// }
}

func TestMockControl() {
	mc, err := mock.NewControl("localhost", 10000, 9000)
	if err != nil {
		log.Fatal(err)
	}

	// if err := mc.Hello(); err != nil {
	// log.Fatal(err)
	// }
	// if w, err := mc.GetWelcome(); err != nil {
	// log.Fatal(err)
	// } else {
	// fmt.Printf("%+v\n", w)
	// }

	if err := mc.SetStation(1); err != nil {
		log.Fatal(err)
	}
	if a, err := mc.GetAnnounce(); err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf("%+v\n", a)
	}

	time.Sleep(time.Second)
	// if err := mc.InvalidCommand(255); err != nil {
	// log.Fatal(err)
	// }
	// time.Sleep(5 * time.Second)
	// if ic, err := mc.GetInvalidCommand(); err != nil {
	// log.Fatal(err)
	// } else {
	// fmt.Printf("%+v\n", ic)
	// }
}
