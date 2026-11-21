package provision

import "context"

func (p *Provisioner) RunAsWatch(ctx context.Context) error {
	return p.run(ctx, false)
}
