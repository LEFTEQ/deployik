import { useParams } from "@tanstack/react-router";

export function ProjectSettingsServices() {
  const { id } = useParams({ strict: false }) as { id: string };
  void id;
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-base font-semibold">Services</h2>
        <p className="text-sm text-muted-foreground">
          Attach a Postgres database to this project. Each environment gets its
          own container + persistent volume. Credentials are revealed on demand;
          external access is via SSH tunnel only.
        </p>
      </div>
      <div className="rounded-2xl border border-dashed border-white/10 p-8 text-center text-sm text-muted-foreground">
        Services panel — wired in Task 19.
      </div>
    </div>
  );
}
