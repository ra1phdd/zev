package service

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeVector encodes exactly dimensions finite float32 values as little-endian
// bytes. A nonpositive dimension, dimension mismatch, or zero norm returns ErrInvalid.
func EncodeVector(vector []float32, dimensions int) ([]byte, error) {
	if _, err := vectorNorm(vector, dimensions); err != nil {
		return nil, err
	}
	blob := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(blob[i*4:], math.Float32bits(value))
	}
	return blob, nil
}

func vectorNorm(vector []float32, dimensions int) (float64, error) {
	if dimensions <= 0 || len(vector) != dimensions {
		return 0, fmt.Errorf("%w: vector dimensions", ErrInvalid)
	}
	var norm float64
	for _, value := range vector {
		v := float64(value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("%w: nonfinite vector", ErrInvalid)
		}
		norm += v * v
	}
	if norm == 0 {
		return 0, fmt.Errorf("%w: zero vector norm", ErrInvalid)
	}
	return math.Sqrt(norm), nil
}

func cosineBlob(query []float32, queryNorm float64, blob []byte, dimensions int) (float64, error) {
	if dimensions <= 0 || len(query) != dimensions || len(blob)%4 != 0 || len(blob)/4 != dimensions {
		return 0, fmt.Errorf("%w: vector blob dimensions", ErrInvalid)
	}
	var dot, norm float64
	for i, value := range query {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:])))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("%w: nonfinite vector blob", ErrInvalid)
		}
		dot += float64(value) * v
		norm += v * v
	}
	if norm == 0 {
		return 0, fmt.Errorf("%w: zero vector blob norm", ErrInvalid)
	}
	return max(-1, min(1, dot/(queryNorm*math.Sqrt(norm)))), nil
}
