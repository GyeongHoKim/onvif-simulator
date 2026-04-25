import { useState } from "react"
import { RiAddLine, RiDeleteBinLine, RiEyeLine, RiEyeOffLine } from "@remixicon/react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldError,
} from "@/components/ui/field"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useSim } from "@/store/simulator"
import * as App from "@/lib/wails/wailsjs/go/main/App"
import { config as cfgNs } from "@/lib/wails/wailsjs/go/models"

const BUILTIN_ROLES = ["Administrator", "Operator", "User", "Extended"]

export function AuthScreen() {
  const config = useSim((s) => s.config)
  const users = useSim((s) => s.users)

  const authEnabled = config?.auth?.enabled ?? false
  const [dialogOpen, setDialogOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  async function toggleAuth(next: boolean) {
    try {
      await App.SetAuthEnabled(next)
      toast.success(`Auth ${next ? "enabled" : "disabled"}`)
      await useSim.getState().refreshConfig()
    } catch (e) {
      toast.error(String(e))
    }
  }

  async function doDelete(username: string) {
    try {
      await App.RemoveUser(username)
      toast.success(`User "${username}" removed`)
      await Promise.all([
        useSim.getState().refreshConfig(),
        useSim.getState().refreshUsers(),
      ])
    } catch (e) {
      toast.error(String(e))
    } finally {
      setConfirmDelete(null)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">Auth</h1>

      <Card>
        <CardContent className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Switch
              checked={authEnabled}
              onCheckedChange={(v) => void toggleAuth(Boolean(v))}
              id="auth-enabled"
            />
            <Label htmlFor="auth-enabled" className="text-sm font-medium">
              Authentication {authEnabled ? "enabled" : "disabled"}
            </Label>
          </div>
        </CardContent>
      </Card>

      {!authEnabled && (
        <Alert>
          <AlertTitle>Auth off</AlertTitle>
          <AlertDescription>
            All ONVIF operations succeed without credentials. Enable auth and
            add at least one user to secure the device.
          </AlertDescription>
        </Alert>
      )}

      <div className="flex items-center justify-between">
        <h2 className="text-lg font-medium">Users</h2>
        <Button onClick={() => setDialogOpen(true)}>
          <RiAddLine data-icon="inline-start" />
          Add user
        </Button>
      </div>

      {users.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No users</EmptyTitle>
            <EmptyDescription>
              Auth is off; enable auth and add a user to secure the device.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Username</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Auth sources</TableHead>
              <TableHead className="w-24 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((u) => (
              <TableRow key={u.username}>
                <TableCell className="font-mono">{u.username}</TableCell>
                <TableCell>{u.roles.join(", ") || "—"}</TableCell>
                <TableCell className="flex gap-1">
                  <Badge variant="secondary">Digest</Badge>
                  <Badge variant="secondary">UsernameToken</Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label="Delete"
                    onClick={() => setConfirmDelete(u.username)}
                  >
                    <RiDeleteBinLine />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <DigestJWTPanels />

      <AddUserDialog open={dialogOpen} onOpenChange={setDialogOpen} />

      <Dialog
        open={confirmDelete !== null}
        onOpenChange={(v) => !v && setConfirmDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete user</DialogTitle>
            <DialogDescription>
              Remove user <code className="font-mono">{confirmDelete}</code>?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (confirmDelete) void doDelete(confirmDelete)
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function AddUserDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [role, setRole] = useState("Administrator")
  const [customRole, setCustomRole] = useState("")
  const [revealPassword, setRevealPassword] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function reset() {
    setUsername("")
    setPassword("")
    setRole("Administrator")
    setCustomRole("")
    setError(null)
  }

  async function submit() {
    const finalRole = role === "Custom" ? customRole : role
    try {
      await App.AddUser(
        cfgNs.UserConfig.createFrom({ username, password, role: finalRole })
      )
      toast.success(`User "${username}" added`)
      await Promise.all([
        useSim.getState().refreshConfig(),
        useSim.getState().refreshUsers(),
      ])
      reset()
      onOpenChange(false)
    } catch (e) {
      setError(String(e))
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) reset()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add user</DialogTitle>
          <DialogDescription>
            User credentials are used for both HTTP Digest and WS-UsernameToken.
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel>Username</FieldLabel>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} />
          </Field>
          <Field>
            <FieldLabel>Password</FieldLabel>
            <div className="flex gap-2">
              <Input
                type={revealPassword ? "text" : "password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <Button
                type="button"
                variant="outline"
                size="icon"
                onClick={() => setRevealPassword((v) => !v)}
                aria-label={revealPassword ? "Hide password" : "Show password"}
              >
                {revealPassword ? <RiEyeOffLine /> : <RiEyeLine />}
              </Button>
            </div>
          </Field>
          <Field>
            <FieldLabel>Role</FieldLabel>
            <Select value={role} onValueChange={(v) => setRole(v ?? "Administrator")}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {BUILTIN_ROLES.map((r) => (
                    <SelectItem key={r} value={r}>
                      {r}
                    </SelectItem>
                  ))}
                  <SelectItem value="Custom">Custom…</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          {role === "Custom" && (
            <Field>
              <FieldLabel>Custom role name</FieldLabel>
              <Input
                value={customRole}
                onChange={(e) => setCustomRole(e.target.value)}
              />
            </Field>
          )}
          {error && (
            <Field data-invalid>
              <FieldError>{error}</FieldError>
            </Field>
          )}
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => void submit()}>Add user</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DigestJWTPanels() {
  const config = useSim((s) => s.config)
  const digest = config?.auth?.digest
  const jwt = config?.auth?.jwt

  return (
    <Card>
      <CardHeader>
        <CardTitle>Digest / JWT</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 text-sm">
        <Alert>
          <AlertTitle>Read-only for MVP</AlertTitle>
          <AlertDescription>
            Edit <code className="font-mono">onvif-simulator.json</code> to
            change these fields.
          </AlertDescription>
        </Alert>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
              Digest
            </div>
            <KV label="Realm" value={digest?.realm || "—"} />
            <KV label="Algorithms" value={(digest?.algorithms ?? []).join(", ") || "—"} />
            <KV label="Nonce TTL" value={digest?.nonce_ttl || "—"} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
              JWT
            </div>
            <KV label="Enabled" value={jwt?.enabled ? "yes" : "no"} />
            <KV label="Issuer" value={jwt?.issuer || "—"} />
            <KV label="Audience" value={jwt?.audience || "—"} />
            <KV label="JWKS URL" value={jwt?.jwks_url || "—"} />
            <KV label="Algorithms" value={(jwt?.algorithms ?? []).join(", ") || "—"} />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function KV({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-2 text-sm">
      <span className="w-28 shrink-0 text-muted-foreground">{label}</span>
      <span className="truncate font-mono">{value}</span>
    </div>
  )
}
