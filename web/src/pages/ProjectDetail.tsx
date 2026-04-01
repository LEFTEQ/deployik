import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams, Link, useNavigate } from '@tanstack/react-router';
import {
  ArrowLeft,
  GitBranch,
  ExternalLink,
  Settings,
  Rocket,
  Globe,
  Key,
  Trash2,
} from 'lucide-react';
import { toast } from 'sonner';
import { api } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { useState } from 'react';

export function ProjectDetail() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: project, isLoading } = useQuery({
    queryKey: ['project', id],
    queryFn: () => api.getProject(id),
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteProject(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      toast.success('Project deleted');
      navigate({ to: '/' });
    },
    onError: (err) => toast.error(err.message),
  });

  if (isLoading) {
    return (
      <div className="p-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="mt-2 h-5 w-64" />
        <Skeleton className="mt-6 h-64 w-full" />
      </div>
    );
  }

  if (!project) {
    return (
      <div className="p-6">
        <p>Project not found</p>
        <Link to="/" className="mt-2 text-sm text-primary hover:underline">
          Back to projects
        </Link>
      </div>
    );
  }

  const previewUrl = `https://${project.name}.preview.example.com`;

  return (
    <div className="p-6">
      {/* Header */}
      <Link
        to="/"
        className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to projects
      </Link>

      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{project.name}</h1>
          <p className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
            <span>
              {project.github_owner}/{project.github_repo}
            </span>
            <span className="flex items-center gap-1">
              <GitBranch className="h-3.5 w-3.5" />
              {project.branch}
            </span>
          </p>
        </div>
        <div className="flex items-center gap-2">
          <a href={previewUrl} target="_blank" rel="noopener noreferrer">
            <Button variant="outline" size="sm">
              <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
              Visit
            </Button>
          </a>
          <Button size="sm">
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deploy
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="deployments" className="mt-6">
        <TabsList>
          <TabsTrigger value="deployments">
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deployments
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings className="mr-1.5 h-3.5 w-3.5" />
            Settings
          </TabsTrigger>
          <TabsTrigger value="domains">
            <Globe className="mr-1.5 h-3.5 w-3.5" />
            Domains
          </TabsTrigger>
          <TabsTrigger value="env">
            <Key className="mr-1.5 h-3.5 w-3.5" />
            Env Vars
          </TabsTrigger>
        </TabsList>

        <TabsContent value="deployments" className="mt-4">
          <DeploymentsTab projectId={id} />
        </TabsContent>

        <TabsContent value="settings" className="mt-4">
          <SettingsTab project={project} onDelete={() => deleteMutation.mutate()} />
        </TabsContent>

        <TabsContent value="domains" className="mt-4">
          <DomainsTab projectId={id} />
        </TabsContent>

        <TabsContent value="env" className="mt-4">
          <EnvVarsTab projectId={id} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function DeploymentsTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const { data: deployments, isLoading } = useQuery({
    queryKey: ['deployments', projectId],
    queryFn: () => api.listDeployments(projectId),
  });

  const deployMutation = useMutation({
    mutationFn: (env: string) =>
      api.triggerDeployment(projectId, { environment: env }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployments', projectId] });
      toast.success('Deployment triggered');
    },
    onError: (err) => toast.error(err.message),
  });

  const statusColor: Record<string, string> = {
    queued: 'bg-muted-foreground',
    building: 'bg-yellow-500',
    deploying: 'bg-blue-500',
    live: 'bg-green-500',
    failed: 'bg-red-500',
    rolled_back: 'bg-orange-500',
    replaced: 'bg-muted-foreground',
  };

  return (
    <div className="space-y-4">
      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={() => deployMutation.mutate('preview')}
          disabled={deployMutation.isPending}
        >
          <Rocket className="mr-1.5 h-3.5 w-3.5" />
          Deploy Preview
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => deployMutation.mutate('production')}
          disabled={deployMutation.isPending}
        >
          <Rocket className="mr-1.5 h-3.5 w-3.5" />
          Deploy Production
        </Button>
      </div>

      {isLoading ? (
        <Card><CardContent className="p-6"><Skeleton className="h-20 w-full" /></CardContent></Card>
      ) : !deployments?.length ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-sm text-muted-foreground">
              No deployments yet. Click deploy to trigger your first build.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {deployments.map((d) => (
            <Link key={d.id} to="/projects/$id/deployments/$did" params={{ id: projectId, did: d.id }}>
              <Card className="transition-colors hover:border-primary/50 cursor-pointer">
                <CardContent className="flex items-center justify-between p-4">
                  <div className="flex items-center gap-3">
                    <div className={`h-2.5 w-2.5 rounded-full ${statusColor[d.status] ?? 'bg-muted-foreground'} ${d.status === 'building' || d.status === 'deploying' ? 'animate-pulse' : ''}`} />
                    <div>
                      <p className="text-sm font-medium">
                        {d.commit_sha ? d.commit_sha.slice(0, 7) : 'pending'}{' '}
                        <span className="font-normal text-muted-foreground">
                          {d.commit_message || d.status}
                        </span>
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {d.environment} &middot; {d.branch} &middot;{' '}
                        {d.build_duration > 0 ? `${d.build_duration}s` : d.status}
                      </p>
                    </div>
                  </div>
                  <Badge variant={d.status === 'live' ? 'default' : d.status === 'failed' ? 'destructive' : 'secondary'}>
                    {d.status}
                  </Badge>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

function SettingsTab({
  project,
  onDelete,
}: {
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
  onDelete: () => void;
}) {
  const queryClient = useQueryClient();
  const [branch, setBranch] = useState(project.branch);
  const [buildCommand, setBuildCommand] = useState(project.build_command);
  const [installCommand, setInstallCommand] = useState(project.install_command);
  const [nodeVersion, setNodeVersion] = useState(project.node_version);

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateProject(project.id, {
        branch,
        build_command: buildCommand,
        install_command: installCommand,
        node_version: nodeVersion,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['project', project.id] });
      toast.success('Settings updated');
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Build Settings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label>Branch</Label>
            <Input
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Build Command</Label>
            <Input
              value={buildCommand}
              onChange={(e) => setBuildCommand(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Install Command</Label>
            <Input
              value={installCommand}
              onChange={(e) => setInstallCommand(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Node.js Version</Label>
            <Select value={nodeVersion} onValueChange={setNodeVersion}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="22">Node.js 22 (LTS)</SelectItem>
                <SelectItem value="20">Node.js 20</SelectItem>
                <SelectItem value="18">Node.js 18</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button
            onClick={() => updateMutation.mutate()}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending ? 'Saving...' : 'Save Settings'}
          </Button>
        </CardContent>
      </Card>

      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="text-base text-destructive">Danger Zone</CardTitle>
        </CardHeader>
        <CardContent>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive">
                <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                Delete Project
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete project?</AlertDialogTitle>
                <AlertDialogDescription>
                  This will stop all running containers and remove the project. This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onDelete}>
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>
    </div>
  );
}

function DomainsTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [newDomain, setNewDomain] = useState('');

  const { data: domains, isLoading } = useQuery({
    queryKey: ['domains', projectId],
    queryFn: () => api.listDomains(projectId),
  });

  const addMutation = useMutation({
    mutationFn: () =>
      api.addDomain(projectId, { domain: newDomain, environment: 'production' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['domains', projectId] });
      setNewDomain('');
      toast.success('Domain added');
    },
    onError: (err) => toast.error(err.message),
  });

  const verifyMutation = useMutation({
    mutationFn: (domainId: string) => api.verifyDomain(projectId, domainId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['domains', projectId] });
      toast.success('Verification complete');
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Add Custom Domain</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            <Input
              placeholder="example.com"
              value={newDomain}
              onChange={(e) => setNewDomain(e.target.value)}
            />
            <Button
              onClick={() => addMutation.mutate()}
              disabled={!newDomain || addMutation.isPending}
            >
              Add
            </Button>
          </div>
        </CardContent>
      </Card>

      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : (
        <div className="space-y-2">
          {domains?.map((d) => (
            <Card key={d.id}>
              <CardContent className="flex items-center justify-between p-4">
                <div>
                  <p className="text-sm font-medium">{d.domain}</p>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Badge variant={d.is_auto ? 'secondary' : 'outline'} className="text-xs">
                      {d.is_auto ? 'auto' : d.environment}
                    </Badge>
                    <span>DNS: {d.dns_verified ? 'verified' : 'pending'}</span>
                    <span>SSL: {d.ssl_status}</span>
                  </div>
                </div>
                {!d.is_auto && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => verifyMutation.mutate(d.id)}
                    disabled={verifyMutation.isPending}
                  >
                    Verify
                  </Button>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

function EnvVarsTab({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [env, setEnv] = useState<'preview' | 'production'>('preview');
  const [rows, setRows] = useState<{ key: string; value: string }[]>([{ key: '', value: '' }]);

  const { data: existingVars, isLoading } = useQuery({
    queryKey: ['envvars', projectId, env],
    queryFn: () => api.listEnvVars(projectId, env),
  });

  const saveMutation = useMutation({
    mutationFn: () =>
      api.bulkSetEnvVars(projectId, {
        environment: env,
        variables: rows.filter((r) => r.key.trim() !== ''),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['envvars', projectId, env] });
      toast.success('Environment variables saved');
    },
    onError: (err) => toast.error(err.message),
  });

  const addRow = () => setRows([...rows, { key: '', value: '' }]);
  const updateRow = (idx: number, field: 'key' | 'value', val: string) => {
    const updated = [...rows];
    updated[idx] = { ...updated[idx]!, [field]: val };
    setRows(updated);
  };
  const removeRow = (idx: number) => setRows(rows.filter((_, i) => i !== idx));

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant={env === 'preview' ? 'default' : 'outline'}
          onClick={() => setEnv('preview')}
        >
          Preview
        </Button>
        <Button
          size="sm"
          variant={env === 'production' ? 'default' : 'outline'}
          onClick={() => setEnv('production')}
        >
          Production
        </Button>
      </div>

      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : existingVars?.length ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Current Variables ({env})</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-1 font-mono text-sm">
              {existingVars.map((v) => (
                <div key={v.id} className="flex gap-2">
                  <span className="text-muted-foreground">{v.key}</span>
                  <span>=</span>
                  <span>{v.value}</span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Set Variables ({env})</CardTitle>
          <CardDescription>
            This will replace all existing variables for this environment
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {rows.map((row, idx) => (
            <div key={idx} className="flex gap-2">
              <Input
                placeholder="KEY"
                value={row.key}
                onChange={(e) => updateRow(idx, 'key', e.target.value.toUpperCase())}
                className="font-mono"
              />
              <Input
                placeholder="value"
                value={row.value}
                onChange={(e) => updateRow(idx, 'value', e.target.value)}
                className="font-mono"
              />
              <Button variant="ghost" size="icon" onClick={() => removeRow(idx)}>
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
          <div className="flex gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={addRow}>
              Add Row
            </Button>
            <Button
              size="sm"
              onClick={() => saveMutation.mutate()}
              disabled={saveMutation.isPending}
            >
              {saveMutation.isPending ? 'Saving...' : 'Save Variables'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
