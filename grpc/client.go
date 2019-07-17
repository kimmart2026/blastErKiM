// Copyright (c) 2019 Minoru Osuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"context"
	"errors"
	"math"

	"github.com/blevesearch/bleve"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	blasterrors "github.com/mosuka/blast/errors"
	"github.com/mosuka/blast/protobuf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   *grpc.ClientConn
	client protobuf.BlastClient
}

func NewContext() (context.Context, context.CancelFunc) {
	baseCtx := context.TODO()
	//return context.WithTimeout(baseCtx, 60*time.Second)
	return context.WithCancel(baseCtx)
}

func NewClient(address string) (*Client, error) {
	ctx, cancel := NewContext()

	//streamRetryOpts := []grpc_retry.CallOption{
	//	grpc_retry.Disable(),
	//}

	//unaryRetryOpts := []grpc_retry.CallOption{
	//	grpc_retry.WithBackoff(grpc_retry.BackoffLinear(100 * time.Millisecond)),
	//	grpc_retry.WithCodes(codes.Unavailable),
	//	grpc_retry.WithMax(100),
	//}

	dialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(math.MaxInt32),
			grpc.MaxCallRecvMsgSize(math.MaxInt32),
		),
		//grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor(streamRetryOpts...)),
		//grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(unaryRetryOpts...)),
	}

	conn, err := grpc.DialContext(ctx, address, dialOpts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		ctx:    ctx,
		cancel: cancel,
		conn:   conn,
		client: protobuf.NewBlastClient(conn),
	}, nil
}

func (c *Client) Cancel() {
	c.cancel()
}

func (c *Client) Close() error {
	c.Cancel()
	if c.conn != nil {
		return c.conn.Close()
	}

	return c.ctx.Err()
}

func (c *Client) GetAddress() string {
	return c.conn.Target()
}

func (c *Client) GetNode(id string, opts ...grpc.CallOption) (map[string]interface{}, error) {
	req := &protobuf.GetNodeRequest{
		Id: id,
	}

	resp, err := c.client.GetNode(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}

	ins, err := protobuf.MarshalAny(resp.NodeConfig)
	nodeConfig := *ins.(*map[string]interface{})

	node := map[string]interface{}{
		"node_config": nodeConfig,
		"state":       resp.State,
	}

	return node, nil
}

func (c *Client) SetNode(id string, nodeConfig map[string]interface{}, opts ...grpc.CallOption) error {
	nodeConfigAny := &any.Any{}
	err := protobuf.UnmarshalAny(nodeConfig, nodeConfigAny)
	if err != nil {
		return err
	}

	req := &protobuf.SetNodeRequest{
		Id:         id,
		NodeConfig: nodeConfigAny,
	}

	_, err = c.client.SetNode(c.ctx, req, opts...)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) DeleteNode(id string, opts ...grpc.CallOption) error {
	req := &protobuf.DeleteNodeRequest{
		Id: id,
	}

	_, err := c.client.DeleteNode(c.ctx, req, opts...)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) GetCluster(opts ...grpc.CallOption) (map[string]interface{}, error) {
	resp, err := c.client.GetCluster(c.ctx, &empty.Empty{}, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}

	ins, err := protobuf.MarshalAny(resp.Cluster)
	cluster := *ins.(*map[string]interface{})

	return cluster, nil
}

func (c *Client) WatchCluster(opts ...grpc.CallOption) (protobuf.Blast_WatchClusterClient, error) {
	req := &empty.Empty{}

	watchClient, err := c.client.WatchCluster(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)
		return nil, errors.New(st.Message())
	}

	return watchClient, nil
}

func (c *Client) Snapshot(opts ...grpc.CallOption) error {
	_, err := c.client.Snapshot(c.ctx, &empty.Empty{})
	if err != nil {
		st, _ := status.FromError(err)

		return errors.New(st.Message())
	}

	return nil
}

func (c *Client) LivenessProbe(opts ...grpc.CallOption) (string, error) {
	resp, err := c.client.LivenessProbe(c.ctx, &empty.Empty{})
	if err != nil {
		st, _ := status.FromError(err)

		return protobuf.LivenessProbeResponse_UNKNOWN.String(), errors.New(st.Message())
	}

	return resp.State.String(), nil
}

func (c *Client) ReadinessProbe(opts ...grpc.CallOption) (string, error) {
	resp, err := c.client.ReadinessProbe(c.ctx, &empty.Empty{})
	if err != nil {
		st, _ := status.FromError(err)

		return protobuf.ReadinessProbeResponse_UNKNOWN.String(), errors.New(st.Message())
	}

	return resp.State.String(), nil
}

func (c *Client) GetValue(key string, opts ...grpc.CallOption) (interface{}, error) {
	req := &protobuf.GetValueRequest{
		Key: key,
	}

	resp, err := c.client.GetValue(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		switch st.Code() {
		case codes.NotFound:
			return nil, blasterrors.ErrNotFound
		default:
			return nil, errors.New(st.Message())
		}
	}

	value, err := protobuf.MarshalAny(resp.Value)

	return value, nil
}

func (c *Client) SetValue(key string, value interface{}, opts ...grpc.CallOption) error {
	valueAny := &any.Any{}
	err := protobuf.UnmarshalAny(value, valueAny)
	if err != nil {
		return err
	}

	req := &protobuf.SetValueRequest{
		Key:   key,
		Value: valueAny,
	}

	_, err = c.client.SetValue(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		switch st.Code() {
		case codes.NotFound:
			return blasterrors.ErrNotFound
		default:
			return errors.New(st.Message())
		}
	}

	return nil
}

func (c *Client) DeleteValue(key string, opts ...grpc.CallOption) error {
	req := &protobuf.DeleteValueRequest{
		Key: key,
	}

	_, err := c.client.DeleteValue(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		switch st.Code() {
		case codes.NotFound:
			return blasterrors.ErrNotFound
		default:
			return errors.New(st.Message())
		}
	}

	return nil
}

func (c *Client) WatchStore(key string, opts ...grpc.CallOption) (protobuf.Blast_WatchStoreClient, error) {
	req := &protobuf.WatchStoreRequest{
		Key: key,
	}

	watchClient, err := c.client.WatchStore(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)
		return nil, errors.New(st.Message())
	}

	return watchClient, nil
}

func (c *Client) GetDocument(id string, opts ...grpc.CallOption) (map[string]interface{}, error) {
	req := &protobuf.GetDocumentRequest{
		Id: id,
	}

	resp, err := c.client.GetDocument(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		switch st.Code() {
		case codes.NotFound:
			return nil, blasterrors.ErrNotFound
		default:
			return nil, errors.New(st.Message())
		}
	}

	ins, err := protobuf.MarshalAny(resp.Fields)
	fields := *ins.(*map[string]interface{})

	return fields, nil
}

func (c *Client) Search(searchRequest *bleve.SearchRequest, opts ...grpc.CallOption) (*bleve.SearchResult, error) {
	// bleve.SearchRequest -> Any
	searchRequestAny := &any.Any{}
	err := protobuf.UnmarshalAny(searchRequest, searchRequestAny)
	if err != nil {
		return nil, err
	}

	req := &protobuf.SearchRequest{
		SearchRequest: searchRequestAny,
	}

	resp, err := c.client.Search(c.ctx, req, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}

	// Any -> bleve.SearchResult
	searchResultInstance, err := protobuf.MarshalAny(resp.SearchResult)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}
	if searchResultInstance == nil {
		return nil, errors.New("nil")
	}
	searchResult := searchResultInstance.(*bleve.SearchResult)

	return searchResult, nil
}

func (c *Client) IndexDocument(docs []map[string]interface{}, opts ...grpc.CallOption) (int, error) {
	stream, err := c.client.IndexDocument(c.ctx, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return -1, errors.New(st.Message())
	}

	for _, doc := range docs {
		id := doc["id"].(string)
		fields := doc["fields"].(map[string]interface{})

		fieldsAny := &any.Any{}
		err := protobuf.UnmarshalAny(&fields, fieldsAny)
		if err != nil {
			return -1, err
		}

		req := &protobuf.IndexDocumentRequest{
			Id:     id,
			Fields: fieldsAny,
		}

		err = stream.Send(req)
		if err != nil {
			return -1, err
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return -1, err
	}

	return int(resp.Count), nil
}

func (c *Client) DeleteDocument(ids []string, opts ...grpc.CallOption) (int, error) {
	stream, err := c.client.DeleteDocument(c.ctx, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return -1, errors.New(st.Message())
	}

	for _, id := range ids {
		req := &protobuf.DeleteDocumentRequest{
			Id: id,
		}

		err := stream.Send(req)
		if err != nil {
			return -1, err
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return -1, err
	}

	return int(resp.Count), nil
}

func (c *Client) GetIndexConfig(opts ...grpc.CallOption) (map[string]interface{}, error) {
	resp, err := c.client.GetIndexConfig(c.ctx, &empty.Empty{}, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}

	indexConfigIntr, err := protobuf.MarshalAny(resp.IndexConfig)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}
	indexConfig := *indexConfigIntr.(*map[string]interface{})

	return indexConfig, nil
}

func (c *Client) GetIndexStats(opts ...grpc.CallOption) (map[string]interface{}, error) {
	resp, err := c.client.GetIndexStats(c.ctx, &empty.Empty{}, opts...)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}

	indexStatsIntr, err := protobuf.MarshalAny(resp.IndexStats)
	if err != nil {
		st, _ := status.FromError(err)

		return nil, errors.New(st.Message())
	}
	indexStats := *indexStatsIntr.(*map[string]interface{})

	return indexStats, nil
}
