-- Create templates table
CREATE TABLE IF NOT EXISTS templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_key VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    prompt TEXT NOT NULL,
    output_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    version INT NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_templates_category_key ON templates(category_key);
CREATE INDEX IF NOT EXISTS idx_templates_is_active ON templates(is_active);

-- Seed 7 default domain templates (Section 7) with full Draft-07 JSON Schemas
INSERT INTO templates (category_key, name, description, prompt, output_schema, version, is_active)
VALUES
(
    'MOM',
    'Minutes of Meeting (MOM)',
    'Ekstraksi agenda, daftar kehadiran, dinamika rapat, keputusan final, dan tabel action items ber-deadline.',
    'Analisis transkrip percakapan berikut ke format Minutes of Meeting (MOM) terstruktur dengan agenda, daftar hadir, konsensus, dan action items ber-assignee dan deadline.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "meeting_title",
    "executive_summary",
    "meeting_dynamics",
    "stakeholder_perspectives",
    "key_decisions",
    "action_items",
    "open_issues"
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
    1,
    TRUE
),
(
    '1_ON_1',
    '1-on-1 Performance & Growth',
    'Penilaian sentimen energi/wellbeing, pencapaian kunci, hambatan operasional, dan rencana aksi pertumbuhan karier.',
    'Ekstraksi intisari percakapan 1-on-1 ke format terstruktur mencakup pulse check wellbeing, milestone, blockers, dan action commitments dua arah.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "wellbeing_assessment",
    "key_wins",
    "blockers",
    "feedback_exchanged",
    "commitments"
  ],
  "properties": {
    "wellbeing_assessment": {
      "type": "object",
      "required": [
        "sentiment_score",
        "summary"
      ],
      "properties": {
        "sentiment_score": {
          "type": "string",
          "enum": [
            "Energetic",
            "Balanced",
            "Stressed",
            "Overwhelmed"
          ]
        },
        "summary": {
          "type": "string"
        }
      }
    },
    "key_wins": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "blockers": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "feedback_exchanged": {
      "type": "object",
      "required": [
        "for_report",
        "for_manager"
      ],
      "properties": {
        "for_report": {
          "type": "array",
          "items": {
            "type": "string"
          }
        },
        "for_manager": {
          "type": "array",
          "items": {
            "type": "string"
          }
        }
      }
    },
    "growth_notes": {
      "type": "string"
    },
    "commitments": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "party",
          "action_item",
          "timeline"
        ],
        "properties": {
          "party": {
            "type": "string",
            "enum": [
              "MANAGER",
              "REPORT"
            ]
          },
          "action_item": {
            "type": "string"
          },
          "timeline": {
            "type": "string"
          }
        }
      }
    }
  }
}$$::jsonb,
    1,
    TRUE
),
(
    'INTERVIEW',
    'Technical & Behavioral Interview Scorecard',
    'Evaluasi kompetensi berbasis metode STAR (Situation, Task, Action, Result), sinyal positif, red flags, dan rekomendasi hiring.',
    'Evaluasi transkrip wawancara kerja ke dalam matriks STAR, breakdown kompetensi teknis, deteksi red flags, dan rekomendasi final penerimaan.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "candidate_name",
    "target_role",
    "recommendation",
    "justification",
    "strengths",
    "concerns",
    "competency_scores"
  ],
  "properties": {
    "candidate_name": {
      "type": "string"
    },
    "target_role": {
      "type": "string"
    },
    "recommendation": {
      "type": "string",
      "enum": [
        "STRONG HIRE",
        "HIRE",
        "LEAN HIRE",
        "LEAN REJECT",
        "STRONG REJECT"
      ]
    },
    "justification": {
      "type": "string"
    },
    "competency_scores": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "competency",
          "rating",
          "evidence"
        ],
        "properties": {
          "competency": {
            "type": "string"
          },
          "rating": {
            "type": "string",
            "enum": [
              "Exceeds",
              "Meets",
              "Needs Work",
              "Unsatisfactory"
            ]
          },
          "evidence": {
            "type": "string"
          }
        }
      }
    },
    "strengths": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "concerns": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "culture_fit_notes": {
      "type": "string"
    },
    "next_round_probes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  }
}$$::jsonb,
    1,
    TRUE
),
(
    'TECH_REVIEW',
    'Engineering RFC & Tech Review',
    'Rangkuman tinjauan arsitektur sistem, catatan Architectural Decision Records (ADR), mitigasi risiko reliabilitas, dan degradasi latensi.',
    'Sintesis percakapan teknis arsitektur ke dalam format RFC/ADR dengan alternatif solusi yang ditolak, konsensus arsitektural, dan rencana mitigasi teknis.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "context",
    "decisions_adopted",
    "rejected_alternatives",
    "nfr_assessment",
    "action_items"
  ],
  "properties": {
    "context": {
      "type": "string"
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
          }
        }
      }
    }
  }
}$$::jsonb,
    1,
    TRUE
),
(
    'SALES_DISCOVERY',
    'B2B Sales Discovery & MEDDPICC',
    'Kualifikasi kesepakatan B2B berbasis kerangka kerja MEDDPICC, Pain Points pelanggan, rasio waktu bicara AE vs Prospek, dan next steps penjualan.',
    'Analisis percakapan sales discovery menggunakan framework MEDDPICC, pain points, timeline evaluasi prospek, dan langkah konfirmasi teknis.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "prospect_company",
    "pain_points",
    "desired_outcomes",
    "meddpicc",
    "objections",
    "next_steps"
  ],
  "properties": {
    "prospect_company": {
      "type": "string"
    },
    "deal_stage_suggested": {
      "type": "string"
    },
    "pain_points": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "pain",
          "cost_of_inaction"
        ],
        "properties": {
          "pain": {
            "type": "string"
          },
          "cost_of_inaction": {
            "type": "string"
          }
        }
      }
    },
    "desired_outcomes": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "meddpicc": {
      "type": "object",
      "required": [
        "metrics",
        "economic_buyer",
        "decision_criteria",
        "champion"
      ],
      "properties": {
        "metrics": {
          "type": "string"
        },
        "economic_buyer": {
          "type": "string"
        },
        "decision_criteria": {
          "type": "string"
        },
        "champion": {
          "type": "string"
        },
        "competition": {
          "type": "string"
        }
      }
    },
    "objections": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "next_steps": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "action",
          "owner",
          "target_date"
        ],
        "properties": {
          "action": {
            "type": "string"
          },
          "owner": {
            "type": "string"
          },
          "target_date": {
            "type": "string"
          }
        }
      }
    }
  }
}$$::jsonb,
    1,
    TRUE
),
(
    'DAILY_STANDUP',
    'Agile Daily Standup & Scrum',
    'Papan status tim 3 kolom (Kemarin, Hari ini, Blockers P0/P1), sprint burndown velocity, dan tindak lanjut cepat pasca standup.',
    'Rangkum pembaruan harian tim scrum ke format matriks individu (Yesterday, Today, Blockers), status kesehatan sprint, dan tindakan huddle lanjutan.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "sprint_health",
    "member_updates",
    "critical_blockers"
  ],
  "properties": {
    "sprint_health": {
      "type": "object",
      "required": [
        "status",
        "summary"
      ],
      "properties": {
        "status": {
          "type": "string",
          "enum": [
            "ON TRACK",
            "AT RISK",
            "BLOCKED"
          ]
        },
        "summary": {
          "type": "string"
        }
      }
    },
    "member_updates": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "member_name",
          "yesterday",
          "today"
        ],
        "properties": {
          "member_name": {
            "type": "string"
          },
          "yesterday": {
            "type": "array",
            "items": {
              "type": "string"
            }
          },
          "today": {
            "type": "array",
            "items": {
              "type": "string"
            }
          },
          "blockers": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "critical_blockers": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "parking_lot_discussions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "topic",
          "participants"
        ],
        "properties": {
          "topic": {
            "type": "string"
          },
          "participants": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    }
  }
}$$::jsonb,
    1,
    TRUE
),
(
    'GENERAL',
    'Executive Briefing & General Discussion',
    'Sintesis memo naratif eksekutif bergaya Cornell Notes, poin kunci kuantitatif empiris, kutipan bernilai strategis, dan agenda tindak lanjut.',
    'Sintesis percakapan umum/diskusi eksekutif ke dalam format Executive Memo bergaya Cornell Notes, empirical takeaways, dan kutipan kunci strategis.',
    $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "executive_overview",
    "core_themes",
    "key_takeaways",
    "notable_quotes"
  ],
  "properties": {
    "executive_overview": {
      "type": "string"
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
    1,
    TRUE
)
ON CONFLICT (category_key) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    prompt = EXCLUDED.prompt,
    output_schema = EXCLUDED.output_schema,
    updated_at = CURRENT_TIMESTAMP;
