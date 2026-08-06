// SPDX-Licence-Identifier: EUPL-1.2

// imap_fake_test.go — a REAL in-process IMAP server
// (github.com/emersion/go-imap/v2/imapserver/imapmemserver, the
// upstream reference in-memory backend) listening on a genuine
// 127.0.0.1:0 socket, used to drive fetchFolder / imapConnect /
// imapConnectWithTLS / imapMsgToRecord end-to-end without a fixed
// port and without any fake/mocked IMAP wire behaviour — SELECT,
// FETCH, APPEND and the UIDVALIDITY bookkeeping all run the real
// emersion protocol state machine. Mirrors pkg/bridge's crib of
// mounting a real service over a mock platform (there: sandbox.Spawn
// against a real process.Service with no container runtime present;
// here: imapclient against a real imapserver with no external MTA).
//
// svc.imapDialOverride (service.go) is the only production seam this
// exercises: fetchFolder swaps in a test dial function instead of
// s.imapConnect. imapConnect itself is never modified, bypassed, or
// weakened — every test that wants to exercise imapConnect's own
// TLS-handshake code calls it directly (see TestImapConnect_* below).

package mail

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"

	imap "github.com/emersion/go-imap/v2"
	imapclient "github.com/emersion/go-imap/v2/imapclient"
	imapserver "github.com/emersion/go-imap/v2/imapserver"
	imapmemserver "github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// generateTestTLSCert returns a throwaway self-signed cert+key for
// 127.0.0.1, scoped to this test binary only, plus an x509.CertPool
// containing that same cert as its sole trust anchor. Test dial calls
// pin RootCAs to this pool instead of setting InsecureSkipVerify —
// the fake server's cert is genuinely verified, just against a
// test-local trust root rather than the system pool (mirrors "add
// the CA to your trust store" for a throwaway in-process listener
// that never talks to anything outside this test binary).
func generateTestTLSCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf}, pool
}

// fakeIMAPServer is a real imapmemserver.Server exposed over a real
// TCP listener — either implicit-TLS (fetchTLS) or plaintext-with-
// STARTTLS (fetchStartTLS), matching the two branches imapConnect /
// imapConnectWithTLS pick between on MailAccount.IMAP.TLS.
type fakeIMAPServer struct {
	addr      string
	user      *imapmemserver.User
	srv       *imapserver.Server
	trustPool *x509.CertPool
}

func newFakeIMAPServer(t *testing.T, username, password string, implicitTLS bool) *fakeIMAPServer {
	t.Helper()
	cert, pool := generateTestTLSCert(t)
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}

	mem := imapmemserver.New()
	user := imapmemserver.NewUser(username, password)
	mem.AddUser(user)

	opts := &imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps: imap.CapSet{imap.CapIMAP4rev1: {}},
	}

	var ln net.Listener
	var err error
	if implicitTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	} else {
		opts.TLSConfig = tlsCfg
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := imapserver.New(opts)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return &fakeIMAPServer{addr: ln.Addr().String(), user: user, srv: srv, trustPool: pool}
}

// createMailbox creates a mailbox (folder) on the fake server so
// SELECT / FETCH have something to operate against. Returns the
// assigned UIDVALIDITY isn't exposed by imapmemserver directly, but
// callers can read it back via a real SELECT after connecting.
func (f *fakeIMAPServer) createMailbox(t *testing.T, name string) {
	t.Helper()
	if err := f.user.Create(name, nil); err != nil {
		t.Fatalf("create mailbox %q: %v", name, err)
	}
}

// appendMessage logs in as a throwaway client and APPENDs raw RFC822
// bytes to mailbox via the real IMAP wire protocol (not a server-
// internal shortcut) so imapMsgToRecord's ENVELOPE parsing is
// exercised against server-computed envelope data.
func (f *fakeIMAPServer) appendMessage(t *testing.T, username, password, mailbox string, raw []byte, flags ...imap.Flag) {
	t.Helper()
	client, err := imapclient.DialTLS(f.addr, &imapclient.Options{
		TLSConfig: &tls.Config{RootCAs: f.trustPool},
	})
	if err != nil {
		t.Fatalf("append-client dial: %v", err)
	}
	defer func() { _ = client.Close() }()
	if err := client.Login(username, password).Wait(); err != nil {
		t.Fatalf("append-client login: %v", err)
	}
	cmd := client.Append(mailbox, int64(len(raw)), &imap.AppendOptions{Flags: flags})
	if _, err := cmd.Write(raw); err != nil {
		t.Fatalf("append write: %v", err)
	}
	if err := cmd.Close(); err != nil {
		t.Fatalf("append close: %v", err)
	}
	if _, err := cmd.Wait(); err != nil {
		t.Fatalf("append wait: %v", err)
	}
}

// dialOverride builds a mail.Service imapDialOverride closure that
// connects through the pre-existing (previously dead-code)
// imapConnectWithTLS seam + performs the same Login step imapConnect
// itself performs — so fetchFolder sees an identical *imapclient.Client
// shape regardless of which dial path produced it.
func (f *fakeIMAPServer) dialOverride() func(*MailAccount, string) (*imapclient.Client, error) {
	return func(acct *MailAccount, _ string) (*imapclient.Client, error) {
		client, err := imapConnectWithTLS(acct, &tls.Config{RootCAs: f.trustPool})
		if err != nil {
			return nil, err
		}
		if err := client.Login(acct.IMAP.User, acct.Auth.Secret).Wait(); err != nil {
			_ = client.Close()
			return nil, err
		}
		return client, nil
	}
}

// splitFakeAddr splits the fake server's "host:port" listener address
// into the (host, port) pair MailAccount.IMAP wants.
func splitFakeAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}
