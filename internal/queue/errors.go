package queue

import "errors"

var (
	ErrInvalidLicensePlate = errors.New("invalid Tunisian license plate format. expected: 'NN TUN NNNN' or 'NNN TUN NNNN'")
)

