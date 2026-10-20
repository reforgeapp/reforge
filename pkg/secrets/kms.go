package secrets

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

var ErrKMSUnavailable = errors.New("KMS wrapping unavailable; check AWS credentials, key policy, approved key ARN and region")

type KMSConfig struct {
	Region         string
	KeyID          string
	PreviousKeyIDs []string
}

type kmsAPI interface {
	Encrypt(context.Context, *kms.EncryptInput, ...func(*kms.Options)) (*kms.EncryptOutput, error)
	Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

type kmsBackend struct {
	client   kmsAPI
	approved map[string]bool
}

var kmsKeyResource = regexp.MustCompile(`^key/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|mrk-[0-9a-f]{32})$`)
var awsAccount = regexp.MustCompile(`^[0-9]{12}$`)
var awsRegion = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

func approvedKMSKeys(c KMSConfig) (map[string]bool, error) {
	if !awsRegion.MatchString(c.Region) || len(c.PreviousKeyIDs) > 32 {
		return nil, errors.New("KMS requires a region and approved immutable key ARNs")
	}
	approved := map[string]bool{}
	for _, id := range append([]string{c.KeyID}, c.PreviousKeyIDs...) {
		a, err := arn.Parse(id)
		if err != nil || a.Service != "kms" || a.Region != c.Region || !awsAccount.MatchString(a.AccountID) || !kmsKeyResource.MatchString(a.Resource) || (a.Partition != "aws" && a.Partition != "aws-us-gov" && a.Partition != "aws-cn") {
			return nil, errors.New("KMS requires approved immutable key ARNs in the configured region")
		}
		approved[id] = true
	}
	return approved, nil
}

func NewKMS(ctx context.Context, c KMSConfig) (*Vault, error) {
	if _, err := approvedKMSKeys(c); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.Region), config.WithHTTPClient(client), config.WithRetryMaxAttempts(2))
	if err != nil {
		return nil, errors.New("AWS credential configuration unavailable; configure a hosted IAM role or standard AWS credential provider")
	}
	if cfg.Credentials == nil {
		return nil, errors.New("AWS credentials unavailable; configure a hosted IAM role or standard AWS credential provider")
	}
	credentials, err := cfg.Credentials.Retrieve(ctx)
	if err != nil || strings.TrimSpace(credentials.AccessKeyID) == "" || strings.TrimSpace(credentials.SecretAccessKey) == "" {
		return nil, errors.New("AWS credentials unavailable; configure a hosted IAM role or standard AWS credential provider")
	}
	service := kms.NewFromConfig(cfg, func(options *kms.Options) {
		options.BaseEndpoint = nil
		options.EndpointResolver = nil
		options.EndpointResolverV2 = kms.NewDefaultEndpointResolverV2()
		options.ClientLogMode = 0
	})
	return newKMS(c, service)
}

func newKMS(c KMSConfig, client kmsAPI) (*Vault, error) {
	approved, err := approvedKMSKeys(c)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrKMSUnavailable
	}
	return &Vault{keyID: c.KeyID, kms: &kmsBackend{client: client, approved: approved}}, nil
}

func kmsContext(b Binding) map[string]string {
	return map[string]string{"reforge:org_id": b.OrgID, "reforge:connection_id": b.ConnectionID, "reforge:credential_version": strconv.FormatInt(b.Version, 10), "reforge:purpose": "connection-data-key-v1"}
}

func kmsFailure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var invalid *types.InvalidCiphertextException
	var wrong *types.IncorrectKeyException
	if errors.As(err, &invalid) || errors.As(err, &wrong) {
		return ErrInvalid
	}
	return ErrKMSUnavailable
}

func (k *kmsBackend) wrap(ctx context.Context, b Binding, keyID string, key []byte) ([]byte, error) {
	result, err := k.client.Encrypt(ctx, &kms.EncryptInput{KeyId: aws.String(keyID), Plaintext: key, EncryptionContext: kmsContext(b), EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault})
	if err != nil {
		return nil, kmsFailure(ctx, err)
	}
	if result == nil || aws.ToString(result.KeyId) != keyID || result.EncryptionAlgorithm != types.EncryptionAlgorithmSpecSymmetricDefault || len(result.CiphertextBlob) == 0 || len(result.CiphertextBlob) > 6144 {
		return nil, ErrKMSUnavailable
	}
	return result.CiphertextBlob, nil
}

func (k *kmsBackend) unwrap(ctx context.Context, b Binding, e Envelope) ([]byte, error) {
	if !k.approved[e.KeyID] || len(e.WrappedKey) == 0 || len(e.WrappedKey) > 6144 {
		return nil, ErrInvalid
	}
	result, err := k.client.Decrypt(ctx, &kms.DecryptInput{KeyId: aws.String(e.KeyID), CiphertextBlob: e.WrappedKey, EncryptionContext: kmsContext(b), EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault})
	if err != nil {
		return nil, kmsFailure(ctx, err)
	}
	if result == nil {
		return nil, ErrInvalid
	}
	if aws.ToString(result.KeyId) != e.KeyID || result.EncryptionAlgorithm != types.EncryptionAlgorithmSpecSymmetricDefault || len(result.Plaintext) != 32 {
		clear(result.Plaintext)
		return nil, ErrInvalid
	}
	return result.Plaintext, nil
}
