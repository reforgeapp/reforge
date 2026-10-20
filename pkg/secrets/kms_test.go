package secrets

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

const firstKMSKey = "arn:aws:kms:ap-southeast-2:123456789012:key/11111111-1111-4111-8111-111111111111"
const nextKMSKey = "arn:aws:kms:ap-southeast-2:123456789012:key/22222222-2222-4222-8222-222222222222"

type kmsRecord struct {
	keyID   string
	dataKey []byte
	context map[string]string
}
type kmsFixture struct {
	mu          sync.Mutex
	records     map[string]kmsRecord
	calls       int
	lastContext map[string]string
	lastPlain   []byte
	fail        error
	wrongID     bool
	wrongLength bool
	block       bool
}

func (f *kmsFixture) Encrypt(ctx context.Context, input *kms.EncryptInput, _ ...func(*kms.Options)) (*kms.EncryptOutput, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail != nil {
		return nil, f.fail
	}
	if len(input.Plaintext) != 32 || input.EncryptionAlgorithm != types.EncryptionAlgorithmSpecSymmetricDefault {
		return nil, errors.New("fixture requires symmetric data key")
	}
	if f.records == nil {
		f.records = map[string]kmsRecord{}
	}
	token := make([]byte, 32)
	rand.Read(token)
	f.records[string(token)] = kmsRecord{aws.ToString(input.KeyId), bytes.Clone(input.Plaintext), input.EncryptionContext}
	f.lastContext = input.EncryptionContext
	id := aws.ToString(input.KeyId)
	if f.wrongID {
		id = nextKMSKey
	}
	return &kms.EncryptOutput{KeyId: aws.String(id), CiphertextBlob: token, EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault}, nil
}
func (f *kmsFixture) Decrypt(ctx context.Context, input *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail != nil {
		return nil, f.fail
	}
	record, ok := f.records[string(input.CiphertextBlob)]
	if !ok || record.keyID != aws.ToString(input.KeyId) || !reflect.DeepEqual(record.context, input.EncryptionContext) || input.EncryptionAlgorithm != types.EncryptionAlgorithmSpecSymmetricDefault {
		return nil, &types.InvalidCiphertextException{Message: aws.String("fixture ciphertext or context mismatch")}
	}
	key := bytes.Clone(record.dataKey)
	id := record.keyID
	if f.wrongID {
		id = nextKMSKey
	}
	if f.wrongLength {
		key = key[:16]
	}
	f.lastPlain = key
	return &kms.DecryptOutput{KeyId: aws.String(id), Plaintext: key, EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault}, nil
}

func TestKMSContextBindingAndRotation(t *testing.T) {
	fixture := &kmsFixture{}
	v, err := newKMS(KMSConfig{Region: "ap-southeast-2", KeyID: firstKMSKey}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{OrgID: "org-a", ConnectionID: "connection-a", Version: 3}
	value := []byte("provider-secret-value")
	envelope, err := v.SealContext(context.Background(), binding, value)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Version != 2 || envelope.KeyID != firstKMSKey || bytes.Contains(envelope.Ciphertext, value) || bytes.Contains(envelope.WrappedKey, value) {
		t.Fatal("KMS envelope unsafe")
	}
	expected := map[string]string{"reforge:org_id": "org-a", "reforge:connection_id": "connection-a", "reforge:credential_version": "3", "reforge:purpose": "connection-data-key-v1"}
	if !reflect.DeepEqual(fixture.lastContext, expected) {
		t.Fatalf("incomplete KMS encryption context: %+v", fixture.lastContext)
	}
	for _, wrong := range []Binding{{"org-b", "connection-a", 3}, {"org-a", "connection-b", 3}, {"org-a", "connection-a", 4}} {
		if _, err = v.OpenContext(context.Background(), wrong, envelope); !errors.Is(err, ErrInvalid) {
			t.Fatalf("cross binding decryption accepted: %v", err)
		}
	}
	plaintext, err := v.Open(binding, envelope)
	if err != nil || !bytes.Equal(plaintext, value) {
		t.Fatalf("KMS open: %v", err)
	}
	if !bytes.Equal(fixture.lastPlain, make([]byte, 32)) {
		t.Fatal("decrypted data key retained after use")
	}
	tampered := envelope
	tampered.Ciphertext = bytes.Clone(envelope.Ciphertext)
	tampered.Ciphertext[0] ^= 1
	if _, err = v.Open(binding, tampered); !errors.Is(err, ErrInvalid) {
		t.Fatal("tampered value accepted")
	}
	rotated, err := newKMS(KMSConfig{Region: "ap-southeast-2", KeyID: nextKMSKey, PreviousKeyIDs: []string{firstKMSKey}}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	rewrapped, err := rotated.RewrapContext(context.Background(), binding, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if rewrapped.KeyID != nextKMSKey || !bytes.Equal(rewrapped.Ciphertext, envelope.Ciphertext) || bytes.Equal(rewrapped.WrappedKey, envelope.WrappedKey) {
		t.Fatal("KMS rewrap changed value or retained old wrapping")
	}
	nextOnly, _ := newKMS(KMSConfig{Region: "ap-southeast-2", KeyID: nextKMSKey}, fixture)
	if plain, err := nextOnly.Open(binding, rewrapped); err != nil || !bytes.Equal(plain, value) {
		t.Fatalf("rewrapped credential unavailable: %v", err)
	}
	calls := fixture.calls
	if _, err = nextOnly.Open(binding, envelope); !errors.Is(err, ErrInvalid) || fixture.calls != calls {
		t.Fatal("unapproved old key reached KMS")
	}
	other := envelope
	other.KeyID = "arn:aws:kms:ap-southeast-2:999999999999:key/11111111-1111-4111-8111-111111111111"
	if _, err = v.Open(binding, other); !errors.Is(err, ErrInvalid) || fixture.calls != calls {
		t.Fatal("unapproved account reached KMS")
	}
	local, _ := New("local", map[string]string{"local": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))})
	localEnvelope, _ := local.Seal(binding, value)
	if _, err = v.Open(binding, localEnvelope); !errors.Is(err, ErrInvalid) {
		t.Fatal("hosted vault accepted operator envelope")
	}
	if _, err = local.Open(binding, envelope); !errors.Is(err, ErrInvalid) {
		t.Fatal("operator vault accepted KMS envelope")
	}
}

func TestKMSFailuresAndCancellation(t *testing.T) {
	binding := Binding{OrgID: "org", ConnectionID: "connection", Version: 1}
	fixture := &kmsFixture{}
	vault, _ := newKMS(KMSConfig{Region: "ap-southeast-2", KeyID: firstKMSKey}, fixture)
	envelope, err := vault.Seal(binding, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	fixture.wrongID = true
	if _, err = vault.Seal(binding, []byte("secret")); !errors.Is(err, ErrKMSUnavailable) {
		t.Fatal("unexpected Encrypt key accepted")
	}
	if _, err = vault.Open(binding, envelope); !errors.Is(err, ErrInvalid) {
		t.Fatal("unexpected Decrypt key accepted")
	}
	fixture.wrongID = false
	fixture.wrongLength = true
	if _, err = vault.Open(binding, envelope); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong data key length accepted")
	}
	fixture.wrongLength = false
	fixture.fail = errors.New("AWS error contains raw provider-secret and credential")
	if _, err = vault.Seal(binding, []byte("secret")); !errors.Is(err, ErrKMSUnavailable) || strings.Contains(err.Error(), "provider-secret") {
		t.Fatal("raw AWS error exposed")
	}
	if _, err = vault.Open(binding, envelope); !errors.Is(err, ErrKMSUnavailable) {
		t.Fatal("KMS outage not classified")
	}
	fixture.fail = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := fixture.calls
	if _, err = vault.SealContext(ctx, binding, []byte("secret")); !errors.Is(err, context.Canceled) || fixture.calls != calls {
		t.Fatal("cancelled seal called KMS")
	}
	fixture.block = true
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err = vault.OpenContext(ctx, binding, envelope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("request cancellation lost: %v", err)
	}
}

func TestKMSKeyApprovalAndStartupCredentials(t *testing.T) {
	for _, cfg := range []KMSConfig{
		{}, {Region: "ap-southeast-2", KeyID: "alias/reforge"}, {Region: "us-east-1", KeyID: firstKMSKey}, {Region: "ap-southeast-2", KeyID: firstKMSKey, PreviousKeyIDs: []string{"alias/old"}}, {Region: "ap-southeast-2", KeyID: strings.Replace(firstKMSKey, ":key/", ":alias/", 1)},
	} {
		if _, err := newKMS(cfg, &kmsFixture{}); err == nil {
			t.Fatal("mutable or unapproved KMS key accepted")
		}
	}
	for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/absent")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/absent")
	cfg := KMSConfig{Region: "ap-southeast-2", KeyID: firstKMSKey}
	if _, err := NewKMS(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "AWS credential") {
		t.Fatalf("missing credentials not actionable: %v", err)
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "fixture-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-secret")
	t.Setenv("AWS_ENDPOINT_URL_KMS", "http://127.0.0.1:1")
	vault, err := NewKMS(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	options := vault.kms.client.(*kms.Client).Options()
	if options.BaseEndpoint != nil || options.EndpointResolver != nil || options.ClientLogMode != 0 {
		t.Fatal("KMS endpoint override or request logging enabled")
	}
}

func TestEnvelopeRejectsAmbiguousBinding(t *testing.T) {
	v, _ := New("local", map[string]string{"local": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))})
	if _, err := v.Seal(Binding{OrgID: "org:connection", ConnectionID: "x", Version: 1}, []byte("secret")); err == nil {
		t.Fatal("ambiguous associated data accepted")
	}
}
