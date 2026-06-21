import { useParams } from "@tanstack/react-router";
import { AppVariableStore } from "@/components/apps/app-variable-store";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function AppVariables() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Variables</h2>
        <p className="text-sm text-muted-foreground">App-level env vars &amp; secrets shared by every member project.</p>
      </div>
      <Card>
        <CardHeader><CardTitle className="text-base">Environment variables</CardTitle></CardHeader>
        <CardContent><AppVariableStore appId={appId} kind="env" /></CardContent>
      </Card>
      <Card>
        <CardHeader><CardTitle className="text-base">Secrets</CardTitle></CardHeader>
        <CardContent><AppVariableStore appId={appId} kind="secret" /></CardContent>
      </Card>
    </div>
  );
}
