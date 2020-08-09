package host

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/csweichel/werft/pkg/werft"

	v1 "github.com/csweichel/werft/pkg/api/v1"
	"github.com/csweichel/werft/pkg/plugin/common"
	log "github.com/sirupsen/logrus"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
)

// Registration registers a plugin
type Registration struct {
	Name    string        `yaml:"name"`
	Command []string      `yaml:"command"`
	Type    []common.Type `yaml:"type"`
	Config  yaml.Node     `yaml:"config"`
}

// Config configures the plugin system
type Config []Registration

// Plugins represents an initialized plugin system
type Plugins struct {
	Errchan chan Error

	stopchan         chan struct{}
	sockets          map[string]string
	repoRegistration RepoRegistrationCallback
	werftService     v1.WerftServiceServer
}

// Stop stops all plugins
func (p *Plugins) Stop() {
	// TODO: backsync stopping using waitgroup
	close(p.stopchan)

	for _, s := range p.sockets {
		os.Remove(s)
	}
}

// Error is passed down the plugins error chan
type Error struct {
	Err error
	Reg *Registration
}

// RepoRegistrationCallback is called when a plugin registers a repo provider
type RepoRegistrationCallback func(host string, repo werft.RepositoryProvider)

// Start starts all configured plugins
func Start(cfg Config, srv v1.WerftServiceServer, repoRegistration RepoRegistrationCallback) (*Plugins, error) {
	errchan, stopchan := make(chan Error), make(chan struct{})

	plugins := &Plugins{
		Errchan:          errchan,
		stopchan:         stopchan,
		sockets:          make(map[string]string),
		repoRegistration: repoRegistration,
		werftService:     srv,
	}

	for _, pr := range cfg {
		err := plugins.startPlugin(pr)
		if err != nil {
			return nil, xerrors.Errorf("cannot start integration plugin %s: %w", pr.Name, err)
		}
	}

	return plugins, nil
}

func (p *Plugins) socketFor(t common.Type) (string, error) {
	switch t {
	case common.TypeIntegration:
		return p.socketForIntegrationPlugin()
	case common.TypeRepository:
		return p.sockerForRepositoryPlugin()
	default:
		return "", xerrors.Errorf("unknown plugin type %s", t)
	}
}

func (p *Plugins) socketForIntegrationPlugin() (string, error) {
	if socket, ok := p.sockets[string(common.TypeIntegration)]; ok {
		return socket, nil
	}

	socketFN := filepath.Join(os.TempDir(), fmt.Sprintf("werft-plugin-integration-%d.sock", time.Now().UnixNano()))
	lis, err := net.Listen("unix", socketFN)
	if err != nil {
		return "", xerrors.Errorf("cannot start integration plugin server: %w", err)
	}
	s := grpc.NewServer()
	v1.RegisterWerftServiceServer(s, p.werftService)
	go func() {
		err := s.Serve(lis)
		if err != nil {
			p.Errchan <- Error{Err: err}
		}
		delete(p.sockets, string(common.TypeIntegration))
	}()
	go func() {
		<-p.stopchan
		s.GracefulStop()
	}()

	p.sockets[string(common.TypeIntegration)] = socketFN
	return socketFN, nil
}

func (p *Plugins) sockerForRepositoryPlugin() (string, error) {
	return filepath.Join(os.TempDir(), fmt.Sprintf("werft-plugin-repo-%d.sock", time.Now().UnixNano())), nil
}

func (p *Plugins) startPlugin(reg Registration) error {
	cfgfile, err := ioutil.TempFile(os.TempDir(), "werft-plugin-cfg")
	if err != nil {
		return xerrors.Errorf("cannot create plugin config: %w", err)
	}
	err = yaml.NewEncoder(cfgfile).Encode(&reg.Config)
	if err != nil {
		return xerrors.Errorf("cannot write plugin config: %w", err)
	}
	err = cfgfile.Close()
	if err != nil {
		return xerrors.Errorf("cannot write plugin config: %w", err)
	}

	for _, t := range reg.Type {
		socket, err := p.socketFor(t)
		if err != nil {
			return err
		}

		pluginName := fmt.Sprintf("%s-%s", reg.Name, t)
		pluginLog := log.WithField("plugin", pluginName)
		stdout := pluginLog.WriterLevel(log.InfoLevel)
		stderr := pluginLog.WriterLevel(log.ErrorLevel)

		var (
			command string
			args    []string
		)
		if len(reg.Command) > 0 {
			command = reg.Command[0]
			args = reg.Command[1:]
		} else {
			command = fmt.Sprintf("werft-plugin-%s", reg.Name)
		}
		args = append(args, string(t), cfgfile.Name(), socket)

		cmd := exec.Command(command, args...)
		cmd.Env = os.Environ()
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		err = cmd.Start()
		if err != nil {
			stdout.Close()
			stderr.Close()
			return err
		}
		pluginLog.Info("plugin started")

		var mayFail bool
		go func() {
			err := cmd.Wait()
			if err != nil && !mayFail {
				p.Errchan <- Error{
					Err: err,
					Reg: &reg,
				}
			}

			stdout.Close()
			stderr.Close()
		}()
		go func() {
			<-p.stopchan
			mayFail = true
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		}()

		if t == common.TypeRepository {
			// repo plugins register repo provider at some point - listen for that
			go p.tryAndRegisterRepoProvider(pluginLog, socket)
		}
	}

	return nil
}

func (p *Plugins) tryAndRegisterRepoProvider(pluginLog *log.Entry, socket string) {
	var (
		t    = time.NewTicker(2 * time.Second)
		conn *grpc.ClientConn
		err  error
	)
	defer t.Stop()
	for {
		conn, err = grpc.Dial("unix://"+socket, grpc.WithInsecure())
		if err != nil {
			pluginLog.Debug("cannot connect to socket (yet)")
			continue
		}
		client := common.NewRepositoryPluginClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		host, err := client.RepoHost(ctx, &common.RepoHostRequest{})
		cancel()
		if err != nil {
			pluginLog.WithError(err).Debug("cannot connect to socket (yet)")
			continue
		}

		defer conn.Close()
		pluginLog.WithField("host", host.Host).Info("registered repo provider")
		p.repoRegistration(host.Host, &pluginHostProvider{client})
		<-p.stopchan

		select {
		case <-t.C:
			continue
		case <-p.stopchan:
			return
		}
	}
}
