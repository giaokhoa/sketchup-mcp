# frozen_string_literal: true

module Giaokhoa
  module SketchupMcp
    module Bridge
      Request = Struct.new(
        :id,
        :session_id,
        :operation,
        :payload,
        :deadline,
        :response_queue,
        :cancelled,
        keyword_init: true
      ) do
        def cancelled?
          cancelled.call
        end
      end
    end
  end
end
