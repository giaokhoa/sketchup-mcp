# frozen_string_literal: true

require 'fileutils'
require 'json'
require 'time'

module Giaokhoa
  module SketchupMcp
    module Bridge
      class Discovery
        def initialize(session_id:, token:, local_app_data: ENV['LOCALAPPDATA'])
          raise ArgumentError, 'LOCALAPPDATA is required for bridge discovery' if local_app_data.to_s.empty?

          @session_id = session_id
          @token = token
          @directory = File.join(local_app_data, 'SketchUpMCP', 'sessions')
          @descriptor_path = File.join(@directory, "#{session_id}.json")
          @temporary_path = File.join(@directory, ".#{session_id}.#{Process.pid}.tmp")
        end

        def publish(host:, port:)
          FileUtils.mkdir_p(@directory)
          descriptor = {
            'bridge' => 'sketchup-mcp',
            'protocol_version' => Protocol::VERSION,
            'session_id' => @session_id,
            'pid' => Process.pid,
            'host' => host,
            'port' => port,
            'token' => @token,
            'started_at' => Time.now.utc.iso8601
          }

          File.open(@temporary_path, 'wb') do |file|
            file.write(JSON.generate(descriptor))
            file.write("\n")
            file.flush
            file.fsync
          end
          File.rename(@temporary_path, @descriptor_path)
          @descriptor_path
        end

        def remove
          File.delete(@descriptor_path) if File.file?(@descriptor_path)
          File.delete(@temporary_path) if File.file?(@temporary_path)
          nil
        rescue SystemCallError
          nil
        end
      end
    end
  end
end
