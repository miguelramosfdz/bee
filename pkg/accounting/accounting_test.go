package accounting_test

import (
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"io/ioutil"
	"testing"
)

const (
	testDisconnectThreshold = 10000
	testPaymentThreshold    = 1000
	testPrice               = 10
)

type Booking struct {
	peer            swarm.Address
	price           int64
	expectedBalance int64
}

func TestAccountingAddBalance(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
		Logger:              logger,
		Store:               store,
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	peer2Addr, err := swarm.ParseHexAddress("00112244")
	if err != nil {
		t.Fatal(err)
	}

	bookings := []Booking{
		{peer: peer1Addr, price: 100, expectedBalance: 100},
		{peer: peer2Addr, price: 200, expectedBalance: 200},
		{peer: peer1Addr, price: 300, expectedBalance: 400},
		{peer: peer1Addr, price: -100, expectedBalance: 300},
		{peer: peer2Addr, price: -1000, expectedBalance: -800},
	}

	for i, booking := range bookings {
		if booking.price < 0 {
			err = acc.Reserve(booking.peer, uint64(booking.price))
			if err != nil {
				t.Fatal(err)
			}
		}

		err = acc.Add(booking.peer, booking.price)
		if err != nil {
			t.Fatal(err)
		}

		balance, err := acc.Balance(booking.peer)
		if err != nil {
			t.Fatal(err)
		}

		if balance != booking.expectedBalance {
			t.Fatalf("balance for peer %v not as expected after booking %d. got %d, wanted %d", booking.peer.String(), i, balance, booking.expectedBalance)
		}

		if booking.price < 0 {
			acc.Release(booking.peer, uint64(booking.price))
		}
	}
}

func TestAccountingAdd_persistentBalances(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)

	store := mock.NewStateStore()
	defer store.Close()

	acc := accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
		Logger:              logger,
		Store:               store,
	})

	peer1Addr, err := swarm.ParseHexAddress("00112233")
	if err != nil {
		t.Fatal(err)
	}

	peer2Addr, err := swarm.ParseHexAddress("00112244")
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Add(peer1Addr, testPrice)
	if err != nil {
		t.Fatal(err)
	}

	err = acc.Add(peer2Addr, 2*testPrice)
	if err != nil {
		t.Fatal(err)
	}

	acc = accounting.NewAccounting(accounting.Options{
		DisconnectThreshold: testDisconnectThreshold,
		PaymentThreshold:    testPaymentThreshold,
		Logger:              logger,
		Store:               store,
	})

	peer1Balance, err := acc.Balance(peer1Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer1Balance != testPrice {
		t.Fatalf("peer1Balance not loaded correctly. got %d, wanted %d", peer1Balance, testPrice)
	}

	peer2Balance, err := acc.Balance(peer2Addr)
	if err != nil {
		t.Fatal(err)
	}

	if peer2Balance != 2*testPrice {
		t.Fatalf("peer2Balance not loaded correctly. got %d, wanted %d", peer2Balance, 2*testPrice)
	}

}
