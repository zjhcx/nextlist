import { sessionText } from "./session-translations"
import { Badge, Button, Heading, HStack, Text, VStack } from "@hope-ui/solid"
import { createSignal, For, onMount, Show } from "solid-js"
import { useFetch, useRouter } from "~/hooks"
import { PEmptyResp, PResp } from "~/types"
import { changeToken, handleResp, handleRespWithoutNotify, r } from "~/utils"

interface Session {
  id: string
  ip: string
  user_agent: string
  created_at: string
  last_active_at: string
  expires_at: string
}

export const Sessions = () => {
  const t = sessionText
  const { to } = useRouter()
  const [sessions, setSessions] = createSignal<Session[]>([])
  const [currentID, setCurrentID] = createSignal("")
  const [loading, fetchSessions] = useFetch(
    (): PResp<{ sessions: Session[]; current_id: string }> =>
      r.get("/me/session/list"),
  )
  const [deleting, remove] = useFetch((id?: string): PEmptyResp =>
    id
      ? r.post("/me/session/delete", { id })
      : r.post("/me/session/delete_others"),
  )
  const refresh = async () => {
    handleRespWithoutNotify(await fetchSessions(), (data) => {
      setSessions(data.sessions)
      setCurrentID(data.current_id)
    })
  }
  const revoke = async (id?: string) => {
    if (!window.confirm(t("sessions_confirm"))) return
    handleResp(await remove(id), () => {
      if (id === currentID()) {
        changeToken("")
        to("/@login")
      } else {
        void refresh()
      }
    })
  }
  onMount(refresh)
  return (
    <VStack w="$full" alignItems="start" spacing="$3">
      <Heading>{t("sessions")}</Heading>
      <HStack spacing="$2">
        <Button loading={loading()} onClick={refresh}>
          {t("sessions_refresh")}
        </Button>
        <Show when={currentID()}>
          <Button
            colorScheme="danger"
            loading={deleting()}
            onClick={() => revoke()}
          >
            {t("sessions_logout_others")}
          </Button>
        </Show>
      </HStack>
      <For each={sessions()} fallback={<Text>{t("sessions_empty")}</Text>}>
        {(session) => (
          <VStack
            w="$full"
            alignItems="start"
            spacing="$1"
            p="$3"
            borderWidth="1px"
            borderRadius="$md"
          >
            <Text css={{ "overflow-wrap": "anywhere" }}>
              {session.user_agent || t("sessions_unknown")}
            </Text>
            <Show when={session.id === currentID()}>
              <Badge colorScheme="success">{t("sessions_current")}</Badge>
            </Show>
            <Text>IP: {session.ip || "—"}</Text>
            <Text>
              {t("sessions_created")}:{" "}
              {new Date(session.created_at).toLocaleString()}
            </Text>
            <Text>
              {t("sessions_active")}:{" "}
              {new Date(session.last_active_at).toLocaleString()}
            </Text>
            <Text>
              {t("sessions_expires")}:{" "}
              {new Date(session.expires_at).toLocaleString()}
            </Text>
            <Button
              colorScheme="danger"
              loading={deleting()}
              onClick={() => revoke(session.id)}
            >
              {t("sessions_logout")}
            </Button>
          </VStack>
        )}
      </For>
    </VStack>
  )
}
