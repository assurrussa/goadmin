package admission

import (
	"context"
	"testing"

	"github.com/assurrussa/goadmin/host"
	"github.com/stretchr/testify/require"
)

func TestUnmountedComponentCannotProvision(t *testing.T) {
	c:=&Component{}
	_,err:=c.Complete(context.Background(),"arbitrary-reference","")
	require.Error(t,err)
}
func TestComponentHasExplicitRoutesAndAccessDependency(t *testing.T) {
	c:=&Component{}
	d:=c.Module().Descriptor()
	require.Equal(t,"admission",d.Key)
	require.Equal(t,[]string{"access"},d.Requires)
	require.Equal(t,[]string{"/admissions"},d.RouteNamespaces)
	require.Equal(t,[]string{"GET /auth/admission"},d.Routes)
}
func TestComponentMissingTrustDependenciesFails(t *testing.T) {
	_,err:=New(context.Background(),Config{})
	require.Error(t,err)
}
func TestGrantPermissionUsesImmutableRoleID(t *testing.T) {
	key,err:=GrantRolePermission("20000000-0000-4000-8000-000000000001")
	require.NoError(t,err)
	require.Equal(t,host.NewPermissionKey("admission","grant-20000000-0000-4000-8000-000000000001"),key)
	for _,value:=range []string{"","super_admin","../roles","arbitrary:permission"}{
		_,err:=GrantRolePermission(value);require.Error(t,err)
	}
}
