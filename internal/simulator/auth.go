package simulator

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// rebuildAuthChain assembles the Digest → UsernameToken [→ JWT] chain used by
// every service handler. Called on startup and after auth-config mutations.
func (s *Simulator) rebuildAuthChain(cfg *config.Config) error {
	digestOpts := auth.DigestOptions{
		Realm: cfg.Auth.Digest.Realm,
	}
	if len(cfg.Auth.Digest.Algorithms) > 0 {
		digestOpts.Algorithms = make([]auth.DigestAlgorithm, 0, len(cfg.Auth.Digest.Algorithms))
		for _, a := range cfg.Auth.Digest.Algorithms {
			digestOpts.Algorithms = append(digestOpts.Algorithms, auth.DigestAlgorithm(a))
		}
	}
	if cfg.Auth.Digest.NonceTTL != "" {
		if d, err := time.ParseDuration(cfg.Auth.Digest.NonceTTL); err == nil {
			digestOpts.NonceTTL = d
		}
	}
	digest := auth.NewDigestAuthenticator(s.store, digestOpts)
	ut := auth.NewUsernameTokenAuthenticator(s.store, auth.UsernameTokenOptions{})

	authenticators := []auth.Authenticator{digest, ut}

	if cfg.Auth.JWT.Enabled {
		jwtAuth, err := buildJWTAuthenticator(&cfg.Auth.JWT)
		if err != nil {
			return fmt.Errorf("jwt: %w", err)
		}
		authenticators = append(authenticators, jwtAuth)
	}

	chain := auth.NewChain(authenticators...)

	s.authMu.Lock()
	s.authChain = chain
	s.authEnabled = cfg.Auth.Enabled
	s.authMu.Unlock()
	return nil
}

func buildJWTAuthenticator(j *config.JWTConfig) (auth.Authenticator, error) {
	opts := auth.JWTOptions{
		Issuer:        j.Issuer,
		Audience:      j.Audience,
		Algorithms:    append([]string(nil), j.Algorithms...),
		UsernameClaim: j.UsernameClaim,
		RolesClaim:    j.RolesClaim,
	}
	if j.ClockSkew != "" {
		if d, err := time.ParseDuration(j.ClockSkew); err == nil {
			opts.ClockSkew = d
		}
	}
	if j.RequireTLS != nil {
		opts.RequireTLS = *j.RequireTLS
	}
	switch {
	case j.JWKSURL != "":
		kf, err := auth.NewJWKSKeyFunc(j.JWKSURL)
		if err != nil {
			return nil, err
		}
		opts.KeyFunc = kf
	case len(j.PublicKeyPEM) > 0:
		blocks := make([][]byte, len(j.PublicKeyPEM))
		for i, s := range j.PublicKeyPEM {
			blocks[i] = []byte(s)
		}
		kf, err := auth.NewStaticKeyFunc(blocks)
		if err != nil {
			return nil, err
		}
		opts.KeyFunc = kf
	default:
		return nil, config.ErrAuthJWTKeyMaterial
	}
	return auth.NewJWTAuthenticator(opts)
}

// currentAuthChain returns a snapshot of (chain, enabled) under the read lock.
func (s *Simulator) currentAuthChain() (auth.Authenticator, bool) {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return s.authChain, s.authEnabled
}

// authorize performs authentication + access-class authorization for one
// operation. When auth is globally disabled, all operations pass.
func (s *Simulator) authorize(ctx context.Context, _ string, r *http.Request, class auth.AccessClass) error {
	chain, enabled := s.currentAuthChain()
	if !enabled {
		return nil
	}
	if class == auth.ClassPreAuth {
		return nil
	}
	principal, err := chain.Authenticate(ctx, r)
	if err != nil {
		return err
	}
	if !auth.DefaultPolicy().Allow(principal, class) {
		forbidden := fmt.Errorf("%w: %s not allowed for class %d",
			auth.ErrForbidden, principal.Username, class)
		return auth.NewChallengeError(forbidden, http.StatusForbidden, nil, auth.OnvifFaultOperationProhibited)
	}
	return nil
}

func (s *Simulator) deviceAuthHook(ctx context.Context, operation string, r *http.Request) error {
	return s.authorize(ctx, operation, r, auth.DeviceOperationClass(operation))
}

func (s *Simulator) mediaAuthHook(ctx context.Context, operation string, r *http.Request) error {
	return s.authorize(ctx, operation, r, auth.MediaOperationClass(operation))
}

func (s *Simulator) eventAuthHook(ctx context.Context, operation string, r *http.Request) error {
	return s.authorize(ctx, operation, r, auth.EventOperationClass(operation))
}
