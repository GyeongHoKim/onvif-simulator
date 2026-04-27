import { useMemo, useState } from "react"
import { RiAddLine, RiEditLine, RiDeleteBinLine } from "@remixicon/react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
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
  Field,
  FieldLabel,
  FieldGroup,
  FieldError,
  FieldDescription,
} from "@/components/ui/field"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { useSim } from "@/store/simulator"
import * as App from "@/lib/wails/wailsjs/go/gui/App"
import { config as cfgNs } from "@/lib/wails/wailsjs/go/models"

type Profile = cfgNs.ProfileConfig

type FormState = {
  name: string
  token: string
  rtsp: string
  encoding: string
  width: string
  height: string
  fps: string
  bitrate: string
  gopLength: string
  snapshotURI: string
  videoSourceToken: string
}

const EMPTY_FORM: FormState = {
  name: "",
  token: "",
  rtsp: "",
  encoding: "H264",
  width: "1280",
  height: "720",
  fps: "30",
  bitrate: "",
  gopLength: "",
  snapshotURI: "",
  videoSourceToken: "",
}

function toFormState(p: Profile): FormState {
  return {
    name: p.name ?? "",
    token: p.token ?? "",
    rtsp: p.rtsp ?? "",
    encoding: p.encoding ?? "H264",
    width: String(p.width ?? ""),
    height: String(p.height ?? ""),
    fps: String(p.fps ?? ""),
    bitrate: p.bitrate ? String(p.bitrate) : "",
    gopLength: p.gop_length ? String(p.gop_length) : "",
    snapshotURI: p.snapshot_uri ?? "",
    videoSourceToken: p.video_source_token ?? "",
  }
}

function formToProfile(f: FormState): Profile {
  return cfgNs.ProfileConfig.createFrom({
    name: f.name,
    token: f.token,
    rtsp: f.rtsp,
    encoding: f.encoding,
    width: Number(f.width || 0),
    height: Number(f.height || 0),
    fps: Number(f.fps || 0),
    bitrate: f.bitrate ? Number(f.bitrate) : 0,
    gop_length: f.gopLength ? Number(f.gopLength) : 0,
    snapshot_uri: f.snapshotURI,
    video_source_token: f.videoSourceToken,
  })
}

export function MediaScreen() {
  const config = useSim((s) => s.config)
  const profiles = useMemo(() => config?.media?.profiles ?? [], [config])

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Profile | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const videoSources = useMemo(() => {
    const seen = new Set<string>()
    for (const p of profiles) {
      const t = p.video_source_token || "VS_DEFAULT"
      seen.add(t)
    }
    return Array.from(seen)
  }, [profiles])

  function openAdd() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(p: Profile) {
    setEditing(p)
    setDialogOpen(true)
  }

  async function doDelete(token: string) {
    try {
      await App.RemoveProfile(token)
      toast.success(`Profile "${token}" deleted`)
      await useSim.getState().refreshConfig()
    } catch (e) {
      toast.error(String(e))
    } finally {
      setConfirmDelete(null)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Media</h1>
        <Button onClick={openAdd}>
          <RiAddLine data-icon="inline-start" />
          Add profile
        </Button>
      </div>

      {profiles.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No profiles yet</EmptyTitle>
            <EmptyDescription>
              Add one to make the simulator answer Media traffic.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Token</TableHead>
              <TableHead>RTSP</TableHead>
              <TableHead>Encoding</TableHead>
              <TableHead>Resolution</TableHead>
              <TableHead>Snapshot URI</TableHead>
              <TableHead className="w-24 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {profiles.map((p) => (
              <TableRow key={p.token}>
                <TableCell>{p.name}</TableCell>
                <TableCell className="font-mono text-xs">{p.token}</TableCell>
                <TableCell className="max-w-[260px] truncate font-mono text-xs">
                  {p.rtsp}
                </TableCell>
                <TableCell>{p.encoding}</TableCell>
                <TableCell className="font-mono text-xs">
                  {p.width}x{p.height}@{p.fps}
                </TableCell>
                <TableCell className="max-w-[200px] truncate font-mono text-xs">
                  {p.snapshot_uri || "—"}
                </TableCell>
                <TableCell className="space-x-1 text-right">
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label="Edit"
                    onClick={() => openEdit(p)}
                  >
                    <RiEditLine />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label="Delete"
                    onClick={() => setConfirmDelete(p.token ?? null)}
                  >
                    <RiDeleteBinLine />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Video sources</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {videoSources.length === 0 ? (
            <span className="text-sm text-muted-foreground">None.</span>
          ) : (
            videoSources.map((t) => (
              <Badge key={t} variant="secondary" className="font-mono">
                {t}
              </Badge>
            ))
          )}
        </CardContent>
      </Card>

      <ProfileDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        initial={editing}
      />
      <Dialog
        open={confirmDelete !== null}
        onOpenChange={(v) => !v && setConfirmDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete profile</DialogTitle>
            <DialogDescription>
              Remove <code className="font-mono">{confirmDelete}</code>?
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

function ProfileDialog({
  open,
  onOpenChange,
  initial,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  initial: Profile | null
}) {
  const isEdit = initial !== null
  const [form, setForm] = useState<FormState>(
    initial ? toFormState(initial) : EMPTY_FORM
  )
  const [error, setError] = useState<string | null>(null)

  // Reset form when dialog opens for a different profile.
  const [lastInitial, setLastInitial] = useState<Profile | null>(initial)
  if (lastInitial !== initial) {
    setLastInitial(initial)
    setForm(initial ? toFormState(initial) : EMPTY_FORM)
    setError(null)
  }

  function update<K extends keyof FormState>(k: K, v: string) {
    setForm((f) => ({ ...f, [k]: v }))
  }

  async function save() {
    setError(null)
    const p = formToProfile(form)
    try {
      if (isEdit) {
        await App.SetProfileRTSP(p.token ?? "", p.rtsp ?? "")
        if (form.snapshotURI || initial?.snapshot_uri) {
          await App.SetProfileSnapshotURI(p.token ?? "", form.snapshotURI)
        }
        await App.SetProfileEncoder(
          p.token ?? "",
          p.encoding ?? "H264",
          Number(p.width ?? 0),
          Number(p.height ?? 0),
          Number(p.fps ?? 0),
          Number(p.bitrate ?? 0),
          Number(p.gop_length ?? 0)
        )
        toast.success(`Profile "${p.token}" updated`)
      } else {
        await App.AddProfile(p)
        toast.success(`Profile "${p.token}" added`)
      }
      await useSim.getState().refreshConfig()
      onOpenChange(false)
    } catch (e) {
      setError(String(e))
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit profile" : "Add profile"}</DialogTitle>
          <DialogDescription>
            Profiles are pass-through — the simulator returns RTSP and snapshot
            URIs verbatim; point them at an external RTP/HTTP server.
          </DialogDescription>
        </DialogHeader>

        <FieldGroup>
          <div className="grid grid-cols-2 gap-4">
            <Field>
              <FieldLabel>Name</FieldLabel>
              <Input
                value={form.name}
                onChange={(e) => update("name", e.target.value)}
                disabled={isEdit}
              />
            </Field>
            <Field>
              <FieldLabel>Token</FieldLabel>
              <Input
                value={form.token}
                onChange={(e) => update("token", e.target.value)}
                disabled={isEdit}
                className="font-mono"
              />
            </Field>
          </div>

          <Field>
            <FieldLabel>RTSP URI</FieldLabel>
            <Input
              value={form.rtsp}
              onChange={(e) => update("rtsp", e.target.value)}
              placeholder="rtsp://..."
              className="font-mono"
            />
            <FieldDescription>Must start with rtsp://.</FieldDescription>
          </Field>

          <div className="grid grid-cols-2 gap-4">
            <Field>
              <FieldLabel>Encoding</FieldLabel>
              <Select
                value={form.encoding}
                onValueChange={(v) => update("encoding", v ?? "H264")}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="H264">H264</SelectItem>
                    <SelectItem value="H265">H265</SelectItem>
                    <SelectItem value="MJPEG">MJPEG</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>Snapshot URI (optional)</FieldLabel>
              <Input
                value={form.snapshotURI}
                onChange={(e) => update("snapshotURI", e.target.value)}
                placeholder="http(s)://..."
                className="font-mono"
              />
            </Field>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <Field>
              <FieldLabel>Width</FieldLabel>
              <Input
                type="number"
                value={form.width}
                onChange={(e) => update("width", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>Height</FieldLabel>
              <Input
                type="number"
                value={form.height}
                onChange={(e) => update("height", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>FPS</FieldLabel>
              <Input
                type="number"
                value={form.fps}
                onChange={(e) => update("fps", e.target.value)}
              />
            </Field>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <Field>
              <FieldLabel>Bitrate (kbps, optional)</FieldLabel>
              <Input
                type="number"
                value={form.bitrate}
                onChange={(e) => update("bitrate", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>GOP length (optional)</FieldLabel>
              <Input
                type="number"
                value={form.gopLength}
                onChange={(e) => update("gopLength", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>Video source token</FieldLabel>
              <Input
                value={form.videoSourceToken}
                onChange={(e) => update("videoSourceToken", e.target.value)}
                className="font-mono"
              />
            </Field>
          </div>

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
          <Button onClick={() => void save()}>Save</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
