# frozen_string_literal: true

module Giaokhoa
  module SketchupMcp
    # Tracks a monotonic revision inside one SketchUp Model#guid identity epoch.
    #
    # Revision semantics:
    # - attaching a different active Model starts revision 0;
    # - a changed Model#guid (including SketchUp's documented GUID rotation after
    #   modifying and saving a model) starts a new identity epoch at revision 0;
    # - non-empty transaction commit, undo and redo increment revision by one;
    # - empty transactions do not increment revision;
    # - observers only update this bookkeeping and never mutate the model.
    class ModelState
      class Observer < Sketchup::ModelObserver
        def initialize(owner)
          super()
          @owner = owner
        end

        def onTransactionCommit(model)
          @owner.transaction_changed(model)
        end

        def onTransactionUndo(model)
          @owner.transaction_changed(model)
        end

        def onTransactionRedo(model)
          @owner.transaction_changed(model)
        end
      end

      def initialize
        @model = nil
        @guid = nil
        @revision = 0
        @observer = Observer.new(self)
      end

      def track(model)
        if @model.equal?(model)
          rebase_guid(model)
          return nil
        end

        detach
        @model = model
        @guid = model.guid
        @revision = 0
        @model.add_observer(@observer)
        nil
      end

      def capture
        model = Sketchup.active_model
        track(model)
        rebase_guid(model)
        [
          model,
          {
            guid: @guid,
            title: model.title.to_s,
            path: model.path.to_s,
            revision: @revision
          }
        ]
      end

      def snapshot
        capture.last
      end

      def transaction_changed(model)
        return nil unless @model.equal?(model)

        return nil if rebase_guid(model)

        @revision += 1
        nil
      end

      def stop
        detach
      end

      private

      def rebase_guid(model)
        current_guid = model.guid
        return false if current_guid == @guid

        @guid = current_guid
        @revision = 0
        true
      end

      def detach
        return unless @model

        @model.remove_observer(@observer)
      rescue StandardError
        nil
      ensure
        @model = nil
      end
    end
  end
end
