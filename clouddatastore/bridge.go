package clouddatastore

import (
	"context"
	"errors"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/datastore"
	w "go.mercari.io/datastore"
	"go.mercari.io/datastore/internal"
	"go.mercari.io/datastore/internal/shared"
	"google.golang.org/api/option"
)

func init() {
	w.FromContext = FromContext
}

var projectID *string

func newClientSettings(opts ...w.ClientOption) *internal.ClientSettings {
	if projectID == nil {
		pID, err := metadata.ProjectID()
		if err != nil {
			// don't check again even if it was failed...
			pID = internal.GetProjectID()
		}
		projectID = &pID
	}
	settings := &internal.ClientSettings{
		ProjectID: *projectID,
	}
	for _, opt := range opts {
		opt.Apply(settings)
	}
	return settings
}

func FromContext(ctx context.Context, opts ...w.ClientOption) (w.Client, error) {
	settings := newClientSettings(opts...)
	origOpts := make([]option.ClientOption, 0, len(opts))
	if len(settings.Scopes) != 0 {
		origOpts = append(origOpts, option.WithScopes(settings.Scopes...))
	}
	if settings.TokenSource != nil {
		origOpts = append(origOpts, option.WithTokenSource(settings.TokenSource))
	}
	if settings.CredentialsFile != "" {
		origOpts = append(origOpts, option.WithCredentialsFile(settings.CredentialsFile))
	}
	if settings.HTTPClient != nil {
		origOpts = append(origOpts, option.WithHTTPClient(settings.HTTPClient))
	}

	client, err := datastore.NewClient(ctx, settings.ProjectID, origOpts...)
	if err != nil {
		return nil, err
	}

	return &datastoreImpl{ctx: ctx, client: client}, nil
}

func IsCloudDatastoreClient(client w.Client) bool {
	_, ok := client.(*datastoreImpl)
	return ok
}

var _ shared.OriginalClientBridge = &originalClientBridgeImpl{}
var _ shared.OriginalTransactionBridge = &originalTransactionBridgeImpl{}
var _ shared.OriginalIteratorBridge = &originalIteratorBridgeImpl{}

type originalClientBridgeImpl struct {
	d *datastoreImpl
}

func (ocb *originalClientBridgeImpl) PutMulti(ctx context.Context, keys []w.Key, psList []w.PropertyList) ([]w.Key, error) {
	origKeys := toOriginalKeys(keys)
	origPss := toOriginalPropertyListList(psList)

	origKeys, err := ocb.d.client.PutMulti(ctx, origKeys, origPss)
	if err != nil {
		return nil, toWrapperError(err)
	}

	return toWrapperKeys(origKeys), nil
}

func (ocb *originalClientBridgeImpl) GetMulti(ctx context.Context, keys []w.Key, psList []w.PropertyList) error {
	origKeys := toOriginalKeys(keys)
	origPss := toOriginalPropertyListList(psList)

	err := ocb.d.client.GetMulti(ctx, origKeys, origPss)
	if err != nil {
		return toWrapperError(err)
	}

	// TODO should be copy? not replace?
	wPss := toWrapperPropertyListList(origPss)
	for idx, wPs := range wPss {
		psList[idx] = wPs
	}

	return nil
}

func (ocb *originalClientBridgeImpl) DeleteMulti(ctx context.Context, keys []w.Key) error {
	origKeys := toOriginalKeys(keys)

	err := ocb.d.client.DeleteMulti(ctx, origKeys)
	if err != nil {
		return toWrapperError(err)
	}

	return nil
}

func (ocb *originalClientBridgeImpl) Run(ctx context.Context, q w.Query) w.Iterator {
	qImpl := q.(*queryImpl)
	iter := ocb.d.client.Run(ctx, qImpl.q)

	// TODO 後々のためにqDumpのshare
	return &iteratorImpl{client: ocb.d, q: qImpl, t: iter, firstError: qImpl.firstError}
}

func (ocb *originalClientBridgeImpl) GetAll(ctx context.Context, q w.Query, psList *[]w.PropertyList) ([]w.Key, error) {
	qImpl := q.(*queryImpl)

	origPss := toOriginalPropertyListList(*psList)
	origKeys, err := ocb.d.client.GetAll(ctx, qImpl.q, &origPss)
	if err != nil {
		return nil, toWrapperError(err)
	}

	wKeys := toWrapperKeys(origKeys)

	// TODO should be copy? not replace?
	*psList = toWrapperPropertyListList(origPss)

	return wKeys, nil
}

type originalTransactionBridgeImpl struct {
	tx *transactionImpl
}

func (otb *originalTransactionBridgeImpl) PutMulti(keys []w.Key, psList []w.PropertyList) ([]w.PendingKey, error) {
	baseTx := getTx(otb.tx.client.ctx)
	if baseTx == nil {
		return nil, errors.New("unexpected context")
	}

	origKeys := toOriginalKeys(keys)
	origPss := toOriginalPropertyListList(psList)

	origPKeys, err := baseTx.PutMulti(origKeys, origPss)
	if err != nil {
		return nil, toWrapperError(err)
	}

	wPKeys := toWrapperPendingKeys(origPKeys)

	return wPKeys, nil
}

func (otb *originalTransactionBridgeImpl) GetMulti(keys []w.Key, psList []w.PropertyList) error {
	baseTx := getTx(otb.tx.client.ctx)
	if baseTx == nil {
		return errors.New("unexpected context")
	}

	origKeys := toOriginalKeys(keys)
	origPss := toOriginalPropertyListList(psList)

	err := baseTx.GetMulti(origKeys, origPss)
	if err != nil {
		return toWrapperError(err)
	}

	// TODO should be copy? not replace?
	wPss := toWrapperPropertyListList(origPss)
	for idx, wPs := range wPss {
		psList[idx] = wPs
	}

	return nil
}

func (otb *originalTransactionBridgeImpl) DeleteMulti(keys []w.Key) error {
	baseTx := getTx(otb.tx.client.ctx)
	if baseTx == nil {
		return errors.New("unexpected context")
	}

	origKeys := toOriginalKeys(keys)

	err := baseTx.DeleteMulti(origKeys)
	if err != nil {
		return toWrapperError(err)
	}

	return nil
}

type originalIteratorBridgeImpl struct {
}

func (oib *originalIteratorBridgeImpl) Next(iter w.Iterator, ps *w.PropertyList) (w.Key, error) {
	iterImpl := iter.(*iteratorImpl)

	origPs := toOriginalPropertyList(*ps)

	origKey, err := iterImpl.t.Next(origPs)
	if err != nil {
		return nil, toWrapperError(err)
	}

	*ps = toWrapperPropertyList(origPs)

	return toWrapperKey(origKey), nil
}
