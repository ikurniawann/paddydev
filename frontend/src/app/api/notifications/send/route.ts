import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { ApiError, requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { query, queryOne } from "@/lib/db";
import { IAM } from "@/lib/iam/prefixes";
import { buildInterviewScheduledMessage } from "@/lib/notifications/candidate-message";
import { candidateStatusEmail, sendEmail } from "@/lib/resend";
import { sendWhatsAppText } from "@/lib/whatsapp";

// Mengirim notifikasi kandidat adalah aksi HR: dibatasi ke peran HR yang
// juga memegang menu Rekrutmen (EPIC-057: akun Non-Rekrutmen ditolak).
const NOTIFY_ROLES = ["super_admin", "admin", "hrd", "hiring_manager"];

const bodySchema = z.object({
  candidate_id: z.string().min(1),
  channel: z.enum(["whatsapp", "email"]).default("whatsapp"),
  template: z.enum(["interview_scheduled", "status_update"]).optional(),
  message: z.string().optional(),
  interview_date: z.string().optional(),
  interview_type: z.string().optional(),
  mode: z.enum(["offline", "online"]).optional(),
  meeting_link: z.string().optional(),
  interviewer_name: z.string().optional(),
});

interface CandidateRow {
  full_name: string;
  email: string | null;
  phone: string | null;
  status: string;
}

export const POST = apiHandler(async (request: NextRequest) => {
  const user = await requireIamMenuPrefix(IAM.hrisRecruitment);
  if (!NOTIFY_ROLES.includes(user.role)) throw ApiError.forbidden();

  const body = await validateBody(request, bodySchema);
  const candidate = await queryOne<CandidateRow>(
    `SELECT full_name, email, phone, status FROM candidates WHERE id = $1`,
    [body.candidate_id]
  );
  if (!candidate) throw ApiError.notFound("Kandidat tidak ditemukan");

  let message = body.message;
  if (!message && body.template === "interview_scheduled") {
    message = buildInterviewScheduledMessage({
      candidateName: candidate.full_name,
      interviewDate: body.interview_date ?? "",
      interviewType: body.interview_type ?? "Interview",
      mode: body.mode ?? "offline",
      meetingLink: body.meeting_link,
      interviewerName: body.interviewer_name,
    });
  }

  let failure = "";
  if (body.channel === "whatsapp") {
    message ||= `Halo ${candidate.full_name}, Status lamaran kamu saat ini: ${candidate.status}`;
    // Fonnte/gateway menerima nomor polos, mis. 6281234567890.
    const result = await sendWhatsAppText({
      target: (candidate.phone ?? "").replace(/\D/g, ""),
      message,
    });
    if (!result.success) failure = result.reason || "Gagal mengirim notifikasi";
  } else {
    const email = candidateStatusEmail(candidate.full_name, candidate.status, message || undefined);
    const sent = await sendEmail({ to: candidate.email ?? "", ...email });
    if (!sent) failure = "Gagal mengirim email";
  }

  await query(
    `INSERT INTO notifications_log (candidate_id, channel, message, status, sent_at)
     VALUES ($1, $2, $3, $4, $5)`,
    [
      body.candidate_id,
      body.channel,
      message || "Notifikasi",
      failure ? "failed" : "sent",
      failure ? null : new Date().toISOString(),
    ]
  );

  if (failure) throw ApiError.server(failure);
  return NextResponse.json({ success: true, message: "Notifikasi terkirim" });
}, "api/notifications/send");
