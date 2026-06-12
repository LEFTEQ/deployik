import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Copy,
  Eye,
  EyeOff,
  FileUp,
  LoaderCircle,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { useState } from "react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { VARIABLE_SCOPE_META } from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { LoadingState } from "@/components/ui/spinner";
import type { VariableScope } from "@/types/api";

export interface VariableStoreProps {
  projectId: string;
  kind: "env" | "secret";
}

export function VariableStore({ projectId, kind }: VariableStoreProps) {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<VariableScope>("shared");
  const [search, setSearch] = useState("");
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [editingVariable, setEditingVariable] = useState<{
    key: string;
    environment: VariableScope;
  } | null>(null);
  const [revealedKeys, setRevealedKeys] = useState<Set<string>>(new Set());

  // Form state for add/edit dialog
  const [formKey, setFormKey] = useState("");
  const [formValue, setFormValue] = useState("");
  const [formEnvironment, setFormEnvironment] =
    useState<VariableScope>("shared");

  // Import state
  const [importText, setImportText] = useState("");
  const [importEnvironment, setImportEnvironment] =
    useState<VariableScope>("shared");

  const isSecret = kind === "secret";
  const storeTitle = isSecret ? "Secrets" : "Environment Variables";
  const storeDescription = isSecret
    ? "Encrypted at rest, never exposed during build, injected only at runtime."
    : "Configuration for your app. NEXT_PUBLIC_* keys are baked into the build.";

  const { data: variables, isLoading } = useQuery({
    queryKey: queryKeys.projectVariables(kind, projectId, scope),
    queryFn: () =>
      isSecret
        ? api.listSecrets(projectId, scope)
        : api.listEnvVars(projectId, scope),
  });

  const invalidateVariables = () =>
    queryClient.invalidateQueries({
      queryKey: queryKeys.projectVariables(kind, projectId),
    });

  const upsertMutation = useMutation({
    mutationFn: (data: { key: string; value: string; environment: string }) =>
      isSecret
        ? api.upsertSecret(projectId, data)
        : api.upsertEnvVar(projectId, data),
    onSuccess: () => {
      invalidateVariables();
      closeAddDialog();
      toast.success(editingVariable ? "Variable updated" : "Variable added");
    },
    onError: (err) => toast.error(err.message),
  });

  const importMutation = useMutation({
    mutationFn: async (entries: { key: string; value: string }[]) => {
      // Parallelize upserts rather than awaiting sequentially — the server
      // accepts each as an additive POST, so order doesn't matter. For a
      // typical .env import (10-50 vars) this cuts wall-clock latency from
      // O(n × round-trip) to a single round-trip.
      const upsertFn = isSecret ? api.upsertSecret : api.upsertEnvVar;
      const results = await Promise.allSettled(
        entries.map((entry) =>
          upsertFn.call(api, projectId, {
            key: entry.key,
            value: entry.value,
            environment: importEnvironment,
          }),
        ),
      );
      const failures = results.filter((r) => r.status === "rejected");
      if (failures.length > 0) {
        throw new Error(
          `${failures.length} of ${entries.length} variables failed to import`,
        );
      }
    },
    onSuccess: () => {
      invalidateVariables();
      setImportDialogOpen(false);
      setImportText("");
      toast.success("Variables imported");
    },
    onError: (err) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (key: string) =>
      isSecret
        ? api.deleteSecret(projectId, key, scope)
        : api.deleteEnvVar(projectId, key, scope),
    onSuccess: () => {
      invalidateVariables();
      toast.success("Variable deleted");
    },
    onError: (err) => toast.error(err.message),
  });

  const filteredVars = (variables ?? []).filter((v) =>
    search ? v.key.toLowerCase().includes(search.toLowerCase()) : true,
  );

  function openAddDialog() {
    setEditingVariable(null);
    setFormKey("");
    setFormValue("");
    setFormEnvironment(scope);
    setAddDialogOpen(true);
  }

  function openEditDialog(key: string, environment: VariableScope) {
    setEditingVariable({ key, environment });
    setFormKey(key);
    setFormValue("");
    setFormEnvironment(environment);
    setAddDialogOpen(true);
  }

  function closeAddDialog() {
    setAddDialogOpen(false);
    setEditingVariable(null);
    setFormKey("");
    setFormValue("");
  }

  function handleSave() {
    const trimmedKey = formKey.trim().toUpperCase();
    if (!trimmedKey || !formValue) return;
    upsertMutation.mutate({
      key: trimmedKey,
      value: formValue,
      environment: formEnvironment,
    });
  }

  function parseEnvText(text: string): { key: string; value: string }[] {
    return text
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line && !line.startsWith("#"))
      .map((line) => {
        const eqIndex = line.indexOf("=");
        if (eqIndex === -1) return null;
        const key = line.slice(0, eqIndex).trim();
        let value = line.slice(eqIndex + 1).trim();
        // Strip surrounding quotes
        if (
          (value.startsWith('"') && value.endsWith('"')) ||
          (value.startsWith("'") && value.endsWith("'"))
        ) {
          value = value.slice(1, -1);
        }
        return key ? { key, value } : null;
      })
      .filter(Boolean) as { key: string; value: string }[];
  }

  function handleImport() {
    const entries = parseEnvText(importText);
    if (entries.length === 0) {
      toast.error("No valid KEY=VALUE pairs found");
      return;
    }
    importMutation.mutate(entries);
  }

  function toggleReveal(key: string) {
    setRevealedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  const parsedImportEntries = importText ? parseEnvText(importText) : [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <h3 className="text-base font-semibold">{storeTitle}</h3>
          <p className="text-sm text-muted-foreground">{storeDescription}</p>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setImportEnvironment(scope);
              setImportDialogOpen(true);
            }}
          >
            <FileUp className="mr-1.5 h-3.5 w-3.5" />
            Import .env
          </Button>
          <Button size="sm" onClick={openAddDialog}>
            <Plus className="mr-1.5 h-3.5 w-3.5" />
            Add Variable
          </Button>
        </div>
      </div>

      {/* Scope filter tabs */}
      <div className="flex flex-wrap gap-1">
        {(Object.keys(VARIABLE_SCOPE_META) as VariableScope[]).map((value) => (
          <Button
            key={value}
            size="sm"
            variant={scope === value ? "default" : "ghost"}
            onClick={() => {
              setScope(value);
              setRevealedKeys(new Set());
            }}
          >
            {VARIABLE_SCOPE_META[value].label}
          </Button>
        ))}
      </div>

      {/* Search */}
      {(variables?.length ?? 0) > 0 && (
        <div className="relative">
          <Search className="absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Filter variables..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-sm"
          />
        </div>
      )}

      {/* Variable list */}
      {isLoading ? (
        <LoadingState
          title={`Loading ${storeTitle.toLowerCase()}...`}
          description={`Fetching stored ${storeTitle.toLowerCase()} for ${VARIABLE_SCOPE_META[scope].label}.`}
          className="min-h-[180px]"
        />
      ) : !filteredVars.length ? (
        <div className="rounded-lg border border-dashed border-border/70 px-4 py-8 text-center text-sm text-muted-foreground">
          {search
            ? "No variables match your filter."
            : `No ${storeTitle.toLowerCase()} for the ${VARIABLE_SCOPE_META[scope].label} scope yet.`}
        </div>
      ) : (
        <div className="divide-y rounded-lg border">
          {filteredVars.map((variable) => {
            const isRevealed = revealedKeys.has(variable.key);
            const deleting =
              deleteMutation.isPending &&
              deleteMutation.variables === variable.key;

            return (
              <div
                key={variable.id}
                className="flex items-center gap-3 px-4 py-3"
              >
                {/* Key + value */}
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="break-all font-mono text-sm text-foreground">
                      {variable.key}
                    </span>
                    <Badge
                      variant="outline"
                      className={
                        VARIABLE_SCOPE_META[variable.environment].badgeClass
                      }
                    >
                      {VARIABLE_SCOPE_META[variable.environment].label}
                    </Badge>
                  </div>
                  <p className="truncate font-mono text-xs text-muted-foreground">
                    {isRevealed ? variable.value : "••••••••"}
                  </p>
                </div>

                {/* Eye toggle */}
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-9 w-9 shrink-0 md:h-8 md:w-8"
                  onClick={() => toggleReveal(variable.key)}
                >
                  {isRevealed ? (
                    <EyeOff className="h-4 w-4" />
                  ) : (
                    <Eye className="h-4 w-4" />
                  )}
                </Button>

                {/* Actions menu */}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-9 w-9 shrink-0 md:h-8 md:w-8"
                    >
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onClick={() =>
                        openEditDialog(variable.key, variable.environment)
                      }
                    >
                      <Pencil className="mr-2 h-3.5 w-3.5" />
                      Edit Value
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() => {
                        navigator.clipboard.writeText(variable.key);
                        toast.success("Key copied");
                      }}
                    >
                      <Copy className="mr-2 h-3.5 w-3.5" />
                      Copy Key
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      disabled={deleting}
                      onClick={() => deleteMutation.mutate(variable.key)}
                    >
                      {deleting ? (
                        <LoaderCircle className="mr-2 h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <Trash2 className="mr-2 h-3.5 w-3.5" />
                      )}
                      Delete
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            );
          })}
        </div>
      )}

      {/* Add / Edit Dialog */}
      <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {editingVariable ? "Edit Variable" : "Add Variable"}
            </DialogTitle>
            <DialogDescription>
              {editingVariable
                ? `Update the value for ${editingVariable.key}.`
                : `Add a new ${isSecret ? "secret" : "environment variable"}.`}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="var-key">Key</Label>
              <Input
                id="var-key"
                placeholder="MY_VARIABLE"
                value={formKey}
                onChange={(e) => setFormKey(e.target.value.toUpperCase())}
                className="font-mono"
                disabled={!!editingVariable}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="var-value">Value</Label>
              <Textarea
                id="var-value"
                placeholder={isSecret ? "secret value" : "value"}
                value={formValue}
                onChange={(e) => setFormValue(e.target.value)}
                className="min-h-[80px] font-mono text-sm"
              />
            </div>
            <div className="space-y-2">
              <Label>Environment</Label>
              <Select
                value={formEnvironment}
                onValueChange={(v) => setFormEnvironment(v as VariableScope)}
                disabled={!!editingVariable}
              >
                <SelectTrigger className="w-full md:w-fit">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(VARIABLE_SCOPE_META) as VariableScope[]).map(
                    (s) => (
                      <SelectItem key={s} value={s}>
                        {VARIABLE_SCOPE_META[s].label}
                      </SelectItem>
                    ),
                  )}
                </SelectContent>
              </Select>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={closeAddDialog}>
              Cancel
            </Button>
            <Button
              onClick={handleSave}
              disabled={
                !formKey.trim() || !formValue || upsertMutation.isPending
              }
            >
              {upsertMutation.isPending ? (
                <>
                  <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  Saving...
                </>
              ) : editingVariable ? (
                "Update"
              ) : (
                "Save"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Import Dialog */}
      <Dialog open={importDialogOpen} onOpenChange={setImportDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Import Variables</DialogTitle>
            <DialogDescription>
              Paste KEY=VALUE pairs, one per line. Lines starting with # are
              ignored. Existing variables with the same key will be overwritten.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Environment</Label>
              <Select
                value={importEnvironment}
                onValueChange={(v) => setImportEnvironment(v as VariableScope)}
              >
                <SelectTrigger className="w-full md:w-fit">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(VARIABLE_SCOPE_META) as VariableScope[]).map(
                    (s) => (
                      <SelectItem key={s} value={s}>
                        {VARIABLE_SCOPE_META[s].label}
                      </SelectItem>
                    ),
                  )}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Content</Label>
              <Textarea
                placeholder={`# Paste your .env content\nDATABASE_URL=postgres://...\nAPI_KEY=sk-...`}
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
                className="min-h-[160px] font-mono text-sm"
              />
            </div>
            {parsedImportEntries.length > 0 && (
              <p className="text-sm text-muted-foreground">
                {parsedImportEntries.length} variable
                {parsedImportEntries.length !== 1 ? "s" : ""} detected
              </p>
            )}
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setImportDialogOpen(false);
                setImportText("");
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={handleImport}
              disabled={
                parsedImportEntries.length === 0 || importMutation.isPending
              }
            >
              {importMutation.isPending ? (
                <>
                  <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  Importing...
                </>
              ) : (
                `Import ${parsedImportEntries.length || ""} Variable${parsedImportEntries.length !== 1 ? "s" : ""}`
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
