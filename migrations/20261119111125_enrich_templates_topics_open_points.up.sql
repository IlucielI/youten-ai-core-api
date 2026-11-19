-- Enrich GENERAL, MOM, and TECH_REVIEW prompt templates with discussion_topics, open_points, and action_items

-- 1. Update MOM Template
UPDATE templates
SET description = 'Ekstraksi agenda, daftar kehadiran, sub pembahasan per topik, dinamika rapat, keputusan final, isu terbuka, dan tabel action items ber-deadline.',
    prompt = 'Analisis transkrip percakapan berikut ke format Minutes of Meeting (MOM) terstruktur dengan agenda, daftar hadir, sub pembahasan dan poin kunci per topik (discussion_topics), konsensus dan keputusan final (key_decisions), isu-isu terbuka (open_points/open_issues), tabel action items dengan properti task, pic, due_date, priority, definition_of_done, dan cost_of_inaction, dinamika rapat (meeting_dynamics) yang mencakup speaker_talk_time, consensus_score, dan sentiment_trajectory, serta perspektif pemangku kepentingan (stakeholder_perspectives) yang mencakup executive_brief, engineering_focus, dan product_delivery. Pastikan seluruh properti wajib tersebut dihasilkan sesuai struktur schema MOM.',
    output_schema = $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "meeting_title",
    "executive_summary",
    "discussion_topics",
    "meeting_dynamics",
    "stakeholder_perspectives",
    "key_decisions",
    "action_items",
    "open_points"
  ],
  "properties": {
    "meeting_title": {
      "type": "string"
    },
    "meeting_goal": {
      "type": "string"
    },
    "executive_summary": {
      "type": "string"
    },
    "discussion_topics": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "topic",
          "key_points"
        ],
        "properties": {
          "topic": {
            "type": "string"
          },
          "key_points": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "meeting_dynamics": {
      "type": "object",
      "required": [
        "speaker_talk_time",
        "consensus_score",
        "sentiment_trajectory"
      ],
      "properties": {
        "speaker_talk_time": {
          "type": "array",
          "items": {
            "type": "object",
            "required": [
              "speaker",
              "percentage",
              "duration_minutes"
            ],
            "properties": {
              "speaker": {
                "type": "string"
              },
              "percentage": {
                "type": "number"
              },
              "duration_minutes": {
                "type": "number"
              }
            }
          }
        },
        "consensus_score": {
          "type": "string"
        },
        "sentiment_trajectory": {
          "type": "string"
        }
      }
    },
    "stakeholder_perspectives": {
      "type": "object",
      "required": [
        "executive_brief",
        "engineering_focus",
        "product_delivery"
      ],
      "properties": {
        "executive_brief": {
          "type": "string"
        },
        "engineering_focus": {
          "type": "string"
        },
        "product_delivery": {
          "type": "string"
        }
      }
    },
    "key_decisions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "decision",
          "root_cause_trigger",
          "contested_points",
          "quantitative_impact",
          "approved_by"
        ],
        "properties": {
          "decision": {
            "type": "string"
          },
          "root_cause_trigger": {
            "type": "string"
          },
          "contested_points": {
            "type": "string"
          },
          "quantitative_impact": {
            "type": "string"
          },
          "approved_by": {
            "type": "string"
          }
        }
      }
    },
    "action_items": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "task",
          "pic",
          "due_date",
          "priority",
          "definition_of_done",
          "cost_of_inaction"
        ],
        "properties": {
          "task": {
            "type": "string"
          },
          "pic": {
            "type": "string"
          },
          "due_date": {
            "type": "string"
          },
          "priority": {
            "type": "string",
            "enum": [
              "P0",
              "P1",
              "P2"
            ]
          },
          "definition_of_done": {
            "type": "string"
          },
          "cost_of_inaction": {
            "type": "string"
          }
        }
      }
    },
    "open_points": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "open_issues": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "next_meeting": {
      "type": "string"
    }
  }
}$$::jsonb,
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE category_key = 'MOM';

-- 2. Update GENERAL Template
UPDATE templates
SET description = 'Sintesis memo eksekutif terstruktur mencakup ringkasan inti, sub pembahasan per topik & poin bahasan, isu terbuka, dan rencana tindak lanjut ber-PIC.',
    prompt = 'Sintesis percakapan umum/diskusi eksekutif ke dalam format Executive Memo terstruktur mencakup ringkasan eksekutif (executive_overview), sub pembahasan dan poin-poin kunci per topik (discussion_topics), isu-isu terbuka yang belum terselesaikan (open_points), tindakan lanjutan ber-PIC (action_items), serta kutipan dan poin kunci strategis.',
    output_schema = $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "executive_overview",
    "discussion_topics",
    "open_points",
    "action_items",
    "key_takeaways"
  ],
  "properties": {
    "executive_overview": {
      "type": "string"
    },
    "discussion_topics": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "topic",
          "key_points"
        ],
        "properties": {
          "topic": {
            "type": "string"
          },
          "key_points": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "open_points": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "action_items": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "task",
          "pic",
          "due_date"
        ],
        "properties": {
          "task": {
            "type": "string"
          },
          "pic": {
            "type": "string"
          },
          "due_date": {
            "type": "string"
          },
          "priority": {
            "type": "string",
            "enum": [
              "P0",
              "P1",
              "P2"
            ]
          }
        }
      }
    },
    "core_themes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "theme",
          "summary_points"
        ],
        "properties": {
          "theme": {
            "type": "string"
          },
          "summary_points": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "key_takeaways": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "notable_quotes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "quote",
          "speaker",
          "context"
        ],
        "properties": {
          "quote": {
            "type": "string"
          },
          "speaker": {
            "type": "string"
          },
          "context": {
            "type": "string"
          }
        }
      }
    },
    "referenced_resources": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  }
}$$::jsonb,
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE category_key = 'GENERAL';

-- 3. Update TECH_REVIEW Template
UPDATE templates
SET description = 'Rangkuman tinjauan arsitektur sistem, RFC/ADR, sub pembahasan teknis, mitigasi risiko, isu terbuka, dan tindakan lanjutan.',
    prompt = 'Sintesis percakapan teknis arsitektur ke dalam format RFC/ADR mencakup konteks arsitektural (context), sub pembahasan teknis dan poin kunci per topik (discussion_topics), keputusan yang diadopsi vs ditolak, penilaian kebutuhan non-fungsional (nfr_assessment) mencakup keamanan, performa, dan reliabilitas, isu terbuka atau risiko yang masih menggantung (open_points), serta rencana aksi mitigasi teknis (action_items). Pastikan seluruh properti wajib tersebut dihasilkan sesuai struktur schema.',
    output_schema = $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "context",
    "discussion_topics",
    "decisions_adopted",
    "rejected_alternatives",
    "nfr_assessment",
    "open_points",
    "action_items"
  ],
  "properties": {
    "context": {
      "type": "string"
    },
    "discussion_topics": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "topic",
          "key_points"
        ],
        "properties": {
          "topic": {
            "type": "string"
          },
          "key_points": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "decisions_adopted": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "decision",
          "technical_justification"
        ],
        "properties": {
          "decision": {
            "type": "string"
          },
          "technical_justification": {
            "type": "string"
          }
        }
      }
    },
    "rejected_alternatives": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "alternative",
          "rejection_reason"
        ],
        "properties": {
          "alternative": {
            "type": "string"
          },
          "rejection_reason": {
            "type": "string"
          }
        }
      }
    },
    "nfr_assessment": {
      "type": "object",
      "required": [
        "security",
        "performance_scalability",
        "reliability_resilience"
      ],
      "properties": {
        "security": {
          "type": "string"
        },
        "performance_scalability": {
          "type": "string"
        },
        "reliability_resilience": {
          "type": "string"
        }
      }
    },
    "technical_debt_impact": {
      "type": "string"
    },
    "open_points": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "action_items": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "task",
          "assignee",
          "target_sprint"
        ],
        "properties": {
          "task": {
            "type": "string"
          },
          "assignee": {
            "type": "string"
          },
          "target_sprint": {
            "type": "string"
          },
          "due_date": {
            "type": "string"
          },
          "priority": {
            "type": "string",
            "enum": [
              "P0",
              "P1",
              "P2"
            ]
          }
        }
      }
    }
  }
}$$::jsonb,
    version = version + 1,
    updated_at = CURRENT_TIMESTAMP
WHERE category_key = 'TECH_REVIEW';
