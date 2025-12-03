package casper

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	matrixpb "github.com/casper-vdb/go-client/pkg/proto"
)

// UploadMatrix uploads a dense matrix to Casper over gRPC streaming, following
// the MatrixService.UploadMatrix contract described in api.md.
//
// - matrixName: logical matrix name, used later in HTTP Matrix/PQ APIs
// - dimension: vector dimensionality (number of float components per row)
// - vectors: flat slice of all vectors concatenated row-wise
// - chunkFloats: number of float32 values per chunk (must be >= dimension)
func (c *Client) UploadMatrix(
	ctx context.Context,
	matrixName string,
	dimension uint32,
	vectors []float32,
	chunkFloats int,
) (*UploadMatrixResult, error) {
	if dimension == 0 {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: "dimension must be greater than 0",
		}
	}

	if len(vectors)%int(dimension) != 0 {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: fmt.Sprintf("vector buffer length %d is not divisible by dimension %d", len(vectors), dimension),
		}
	}

	if chunkFloats < int(dimension) {
		chunkFloats = int(dimension)
	}

	totalFloats := len(vectors)
	totalChunks := (totalFloats + chunkFloats - 1) / chunkFloats

	conn, err := grpc.DialContext(
		ctx,
		c.grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: "failed to connect to gRPC endpoint",
			Cause:   err,
		}
	}
	defer conn.Close()

	client := matrixpb.NewMatrixServiceClient(conn)
	stream, err := client.UploadMatrix(ctx)
	if err != nil {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: "failed to open UploadMatrix stream",
			Cause:   err,
		}
	}

	// Send header first.
	maxVectorsPerChunk := chunkFloats / int(dimension)
	if maxVectorsPerChunk < 1 {
		maxVectorsPerChunk = 1
	}
	header := &matrixpb.MatrixHeader{
		Name:               matrixName,
		Dimension:          dimension,
		TotalChunks:        uint32(totalChunks),
		MaxVectorsPerChunk: uint32(maxVectorsPerChunk),
	}
	if err := stream.Send(&matrixpb.UploadMatrixRequest{
		Payload: &matrixpb.UploadMatrixRequest_Header{Header: header},
	}); err != nil {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: "failed to send matrix header",
			Cause:   err,
		}
	}

	// Then send all chunks sequentially.
	for chunkIdx := 0; chunkIdx < totalChunks; chunkIdx++ {
		start := chunkIdx * chunkFloats
		end := start + chunkFloats
		if end > totalFloats {
			end = totalFloats
		}
		slice := vectors[start:end]

		data := &matrixpb.MatrixData{
			ChunkIndex: uint32(chunkIdx + 1),
			Vector:     slice,
		}
		if err := stream.Send(&matrixpb.UploadMatrixRequest{
			Payload: &matrixpb.UploadMatrixRequest_Data{Data: data},
		}); err != nil {
			return nil, &Error{
				Kind:    ErrorKindClient,
				Message: "failed to send matrix data chunk",
				Cause:   err,
			}
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: "UploadMatrix stream failed",
			Cause:   err,
		}
	}

	return &UploadMatrixResult{
		Success:      true,
		Message:      fmt.Sprintf("Successfully uploaded %d vectors in %d chunks", resp.TotalVectors, resp.TotalChunks),
		TotalVectors: resp.TotalVectors,
		TotalChunks:  resp.TotalChunks,
	}, nil
}
